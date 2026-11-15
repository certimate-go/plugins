package integration

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	purgeAWSAccessKeyID     = "fixture-purge-aws-id"
	purgeAWSSecretAccessKey = "fixture-purge-aws-secret"
	purgeAWSRegion          = "us-fixture-1"
	purgeAWSIMDSRole        = "fixture-imds-role"
	purgeAWSIMDSAccessKeyID = "fixture-imds-aws-id"
	purgeAWSIMDSSecretKey   = "fixture-imds-aws-secret"
)

var (
	purgeAWSPlugins = []string{"aws-acm-purger", "aws-iam-purger"}
	purgeAWSHosts   = []string{"acm." + purgeAWSRegion + ".amazonaws.com", "iam.amazonaws.com"}
)

type purgeAWSCert struct {
	arn      string
	notAfter time.Time
	origin   string
	noExpiry bool
}

type purgeAWSIAMCert struct {
	name     string
	path     string
	expiry   time.Time
	noExpiry bool
}

type purgeAWSACMService struct {
	certs      map[string]*purgeAWSCert
	order      []string
	deletes    []string
	lists      int
	pageSize   int
	nextTokens []string
	vanishARN  string
	inUseARN   string
	denyARN    string
	listFail   bool
}

type purgeAWSIAMService struct {
	certs        map[string]*purgeAWSIAMCert
	order        []string
	deletes      []string
	lists        int
	pageSize     int
	markers      []string
	pathPrefixes []string
	vanishName   string
	conflictName string
	denyName     string
	listFail     bool
}

type purgeAWSFixture struct {
	mu   sync.Mutex
	mode string

	acm            purgeAWSACMService
	iam            purgeAWSIAMService
	acmCredentials []string
	iamCredentials []string

	imdsTokens     int
	imdsRoleLists  int
	imdsCredsFetch int
}

func newPurgeAWSFixture(t *testing.T) *purgeAWSFixture {
	t.Helper()
	f := &purgeAWSFixture{
		acm: purgeAWSACMService{certs: map[string]*purgeAWSCert{}},
		iam: purgeAWSIAMService{certs: map[string]*purgeAWSIAMCert{}},
	}
	newB4CloudProxy(t, purgeAWSHosts, http.HandlerFunc(f.serve))

	imds := httptest.NewServer(http.HandlerFunc(f.serveIMDS))
	t.Cleanup(imds.Close)
	t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", imds.URL)
	return f
}

func (f *purgeAWSFixture) acmSeed(arn string, daysToExpiry float64) {
	f.acm.seed(arn, daysToExpiry, "", false)
}

func (f *purgeAWSFixture) acmSeedExotic(arn string, daysToExpiry float64, origin string, noExpiry bool) {
	f.acm.seed(arn, daysToExpiry, origin, noExpiry)
}

func (s *purgeAWSACMService) seed(arn string, daysToExpiry float64, origin string, noExpiry bool) {
	if _, exists := s.certs[arn]; !exists {
		s.order = append(s.order, arn)
	}
	s.certs[arn] = &purgeAWSCert{
		arn:      arn,
		notAfter: time.Now().Add(time.Duration(-daysToExpiry * 24 * float64(time.Hour))),
		origin:   origin,
		noExpiry: noExpiry,
	}
}

func (s *purgeAWSACMService) live() []*purgeAWSCert {
	items := make([]*purgeAWSCert, 0, len(s.order))
	for _, arn := range s.order {
		if cert, exists := s.certs[arn]; exists {
			items = append(items, cert)
		}
	}
	return items
}

func (s *purgeAWSACMService) reset() {
	s.certs = map[string]*purgeAWSCert{}
	s.order = nil
	s.deletes = nil
	s.lists = 0
	s.pageSize = 0
	s.nextTokens = nil
	s.vanishARN = ""
	s.inUseARN = ""
	s.denyARN = ""
	s.listFail = false
}

func (s *purgeAWSACMService) remaining() []string {
	ids := make([]string, 0, len(s.certs))
	for _, arn := range s.order {
		if _, exists := s.certs[arn]; exists {
			ids = append(ids, arn)
		}
	}
	return ids
}

func (f *purgeAWSFixture) iamSeed(name, path string, daysToExpiry float64) {
	f.iam.seed(name, path, daysToExpiry, false)
}

func (s *purgeAWSIAMService) seed(name, path string, daysToExpiry float64, noExpiry bool) {
	if _, exists := s.certs[name]; !exists {
		s.order = append(s.order, name)
	}
	s.certs[name] = &purgeAWSIAMCert{
		name:     name,
		path:     path,
		expiry:   time.Now().Add(time.Duration(-daysToExpiry * 24 * float64(time.Hour))),
		noExpiry: noExpiry,
	}
}

func (s *purgeAWSIAMService) live(prefix string) []*purgeAWSIAMCert {
	items := make([]*purgeAWSIAMCert, 0, len(s.order))
	for _, name := range s.order {
		if cert, exists := s.certs[name]; exists && strings.HasPrefix(cert.path, prefix) {
			items = append(items, cert)
		}
	}
	return items
}

func (s *purgeAWSIAMService) reset() {
	s.certs = map[string]*purgeAWSIAMCert{}
	s.order = nil
	s.deletes = nil
	s.lists = 0
	s.pageSize = 0
	s.markers = nil
	s.pathPrefixes = nil
	s.vanishName = ""
	s.conflictName = ""
	s.denyName = ""
	s.listFail = false
}

func (s *purgeAWSIAMService) remaining() []string {
	ids := make([]string, 0, len(s.certs))
	for _, name := range s.order {
		if _, exists := s.certs[name]; exists {
			ids = append(ids, name)
		}
	}
	return ids
}

func purgeAWSCredentialID(r *http.Request) string {
	authorization := r.Header.Get("Authorization")
	start := strings.Index(authorization, "Credential=")
	if start < 0 {
		return ""
	}
	rest := authorization[start+len("Credential="):]
	if end := strings.Index(rest, "/"); end >= 0 {
		return rest[:end]
	}
	return rest
}

func (f *purgeAWSFixture) apiError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": "com.amazonaws.fixture#" + code, "message": message})
}

func (f *purgeAWSFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	credentialID := purgeAWSCredentialID(r)
	if credentialID != purgeAWSAccessKeyID && credentialID != purgeAWSIMDSAccessKeyID {
		f.apiError(w, http.StatusForbidden, "InvalidClientTokenIdException", "unexpected credential "+credentialID)
		return
	}

	switch r.Host {
	case "acm." + purgeAWSRegion + ".amazonaws.com":
		f.acmCredentials = append(f.acmCredentials, credentialID)
		f.serveACM(w, r)
	case "iam.amazonaws.com":
		f.iamCredentials = append(f.iamCredentials, credentialID)
		f.serveIAM(w, r)
	default:
		http.Error(w, "unexpected fixture host "+r.Host, http.StatusBadRequest)
	}
}

func (f *purgeAWSFixture) serveACM(w http.ResponseWriter, r *http.Request) {
	if f.mode == "auth-failure" {
		f.apiError(w, http.StatusForbidden, "InvalidClientTokenIdException", "fixture auth-failure")
		return
	}

	target := r.Header.Get("X-Amz-Target")
	var body struct {
		NextToken      string  `json:"NextToken"`
		MaxItems       float64 `json:"MaxItems"`
		CertificateArn string  `json:"CertificateArn"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	switch {
	case strings.HasSuffix(target, "ListCertificates"):
		if f.acm.listFail {
			f.apiError(w, http.StatusForbidden, "AccessDeniedException", "fixture list failure")
			return
		}
		if body.MaxItems != 1000 {
			f.apiError(w, http.StatusBadRequest, "InvalidParameterException", fmt.Sprintf("MaxItems %v, want 1000", body.MaxItems))
			return
		}
		f.acm.lists++
		if body.NextToken != "" {
			f.acm.nextTokens = append(f.acm.nextTokens, body.NextToken)
		}
		live := f.acm.live()
		page := live
		nextToken := ""
		if f.acm.pageSize > 0 {
			index := 0
			if body.NextToken != "" {
				if _, err := fmt.Sscanf(body.NextToken, "acm-page-%d", &index); err != nil {
					f.apiError(w, http.StatusBadRequest, "InvalidParameterException", "bad token "+body.NextToken)
					return
				}
			}
			start := index * f.acm.pageSize
			if start > len(live) {
				start = len(live)
			}
			end := start + f.acm.pageSize
			if end > len(live) {
				end = len(live)
			}
			page = live[start:end]
			if end < len(live) {
				nextToken = fmt.Sprintf("acm-page-%d", index+1)
			}
		}
		items := make([]map[string]any, 0, len(page))
		for _, cert := range page {
			item := map[string]any{"CertificateArn": cert.arn}
			if !cert.noExpiry {
				item["NotAfter"] = float64(cert.notAfter.Unix())
			}
			if cert.origin != "" {
				item["CertificateKeyPairOrigin"] = cert.origin
			}
			items = append(items, item)
		}
		response := map[string]any{"CertificateSummaryList": items}
		if nextToken != "" {
			response["NextToken"] = nextToken
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_ = json.NewEncoder(w).Encode(response)
	case strings.HasSuffix(target, "DeleteCertificate"):
		if body.CertificateArn == "" {
			f.apiError(w, http.StatusBadRequest, "InvalidParameterException", "delete without CertificateArn")
			return
		}
		if body.CertificateArn == f.acm.vanishARN {
			f.apiError(w, http.StatusNotFound, "ResourceNotFoundException", "certificate vanished between list and delete")
			return
		}
		if body.CertificateArn == f.acm.inUseARN {
			f.apiError(w, http.StatusBadRequest, "ResourceInUseException", "certificate is in use")
			return
		}
		if body.CertificateArn == f.acm.denyARN {
			f.apiError(w, http.StatusForbidden, "AccessDeniedException", "fixture delete denied")
			return
		}
		if _, exists := f.acm.certs[body.CertificateArn]; !exists {
			f.apiError(w, http.StatusNotFound, "ResourceNotFoundException", "unknown certificate")
			return
		}
		delete(f.acm.certs, body.CertificateArn)
		f.acm.deletes = append(f.acm.deletes, body.CertificateArn)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	default:
		f.apiError(w, http.StatusBadRequest, "InvalidActionException", "unexpected target "+target)
	}
}

type purgeAWSIAMError struct {
	XMLName xml.Name `xml:"ErrorResponse"`
	Error   struct {
		Type    string `xml:"Type"`
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	} `xml:"Error"`
	RequestID string `xml:"RequestId"`
}

func (f *purgeAWSFixture) iamError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	payload := purgeAWSIAMError{RequestID: "fixture"}
	payload.Error.Type = "Sender"
	payload.Error.Code = code
	payload.Error.Message = message
	_ = xml.NewEncoder(w).Encode(payload)
}

func (f *purgeAWSFixture) serveIAM(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if f.mode == "auth-failure" {
		f.iamError(w, http.StatusForbidden, "InvalidClientTokenId", "fixture auth-failure")
		return
	}
	if version := r.PostForm.Get("Version"); version != "2010-05-08" {
		f.iamError(w, http.StatusBadRequest, "InvalidVersion", version)
		return
	}

	switch r.PostForm.Get("Action") {
	case "ListServerCertificates":
		if f.iam.listFail {
			f.iamError(w, http.StatusForbidden, "AccessDenied", "fixture list failure")
			return
		}
		if maxItems := r.PostForm.Get("MaxItems"); maxItems != "1000" {
			f.iamError(w, http.StatusBadRequest, "InvalidParameterValue", "MaxItems "+maxItems)
			return
		}
		f.iam.lists++
		prefix := r.PostForm.Get("PathPrefix")
		f.iam.pathPrefixes = append(f.iam.pathPrefixes, prefix)
		if marker := r.PostForm.Get("Marker"); marker != "" {
			f.iam.markers = append(f.iam.markers, marker)
		}
		live := f.iam.live(prefix)
		page := live
		truncated := false
		marker := ""
		if f.iam.pageSize > 0 {
			index := 0
			if received := r.PostForm.Get("Marker"); received != "" {
				if _, err := fmt.Sscanf(received, "iam-page-%d", &index); err != nil {
					f.iamError(w, http.StatusBadRequest, "InvalidParameterValue", "bad marker "+received)
					return
				}
			}
			start := index * f.iam.pageSize
			if start > len(live) {
				start = len(live)
			}
			end := start + f.iam.pageSize
			if end > len(live) {
				end = len(live)
			}
			page = live[start:end]
			if end < len(live) {
				truncated = true
				marker = fmt.Sprintf("iam-page-%d", index+1)
			}
		}
		var members strings.Builder
		for _, cert := range page {
			expiration := ""
			if !cert.noExpiry {
				expiration = "<Expiration>" + cert.expiry.UTC().Format(time.RFC3339) + "</Expiration>"
			}
			fmt.Fprintf(&members, "<member><Path>%s</Path><ServerCertificateName>%s</ServerCertificateName>%s</member>", cert.path, cert.name, expiration)
		}
		markerElement := ""
		if truncated {
			markerElement = "<Marker>" + marker + "</Marker>"
		}
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintf(w, `<ListServerCertificatesResponse><ListServerCertificatesResult><IsTruncated>%t</IsTruncated>%s<ServerCertificateMetadataList>%s</ServerCertificateMetadataList></ListServerCertificatesResult><ResponseMetadata><RequestId>fixture</RequestId></ResponseMetadata></ListServerCertificatesResponse>`, truncated, markerElement, members.String())
	case "DeleteServerCertificate":
		name := r.PostForm.Get("ServerCertificateName")
		if name == f.iam.vanishName {
			f.iamError(w, http.StatusNotFound, "NoSuchEntity", "certificate vanished between list and delete")
			return
		}
		if name == f.iam.conflictName {
			f.iamError(w, http.StatusConflict, "DeleteConflict", "certificate is referenced")
			return
		}
		if name == f.iam.denyName {
			f.iamError(w, http.StatusForbidden, "AccessDenied", "fixture delete denied")
			return
		}
		if _, exists := f.iam.certs[name]; !exists {
			f.iamError(w, http.StatusNotFound, "NoSuchEntity", "unknown certificate")
			return
		}
		delete(f.iam.certs, name)
		f.iam.deletes = append(f.iam.deletes, name)
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<DeleteServerCertificateResponse><ResponseMetadata><RequestId>fixture</RequestId></ResponseMetadata></DeleteServerCertificateResponse>`)
	default:
		f.iamError(w, http.StatusBadRequest, "InvalidAction", r.PostForm.Get("Action"))
	}
}

func (f *purgeAWSFixture) serveIMDS(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case r.Method == http.MethodPut && r.URL.Path == "/latest/api/token":
		f.imdsTokens++
		w.Header().Set("x-aws-ec2-metadata-token", "fixture-imds-token")
	case r.Method == http.MethodGet && r.URL.Path == "/latest/meta-data/iam/security-credentials/":
		f.imdsRoleLists++
		fmt.Fprint(w, purgeAWSIMDSRole+"\n")
	case r.Method == http.MethodGet && r.URL.Path == "/latest/meta-data/iam/security-credentials/"+purgeAWSIMDSRole:
		f.imdsCredsFetch++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Code":            "Success",
			"Type":            "AWS-HMAC",
			"AccessKeyId":     purgeAWSIMDSAccessKeyID,
			"SecretAccessKey": purgeAWSIMDSSecretKey,
			"Token":           "fixture-imds-session-token",
			"Expiration":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	default:
		http.Error(w, "unexpected imds request "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

func (f *purgeAWSFixture) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
}

func TestPurgeWorkflowAWS(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for purge workflow verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_PURGE_BINARIES") == "" {
		runPurgeAWSInDocker(t)
		return
	}
	runPurgeAWSScenarios(t, os.Getenv("CERTIMATE_PURGE_BINARIES"))
}

func runPurgeAWSScenarios(t *testing.T, binaryDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fixture := newPurgeAWSFixture(t)
	core := startPurgeCore(t, binaryDir, purgeAWSPlugins)

	for _, provider := range purgeAWSPlugins {
		core.assertCatalogEntry(provider, "purge", true)
	}
	if envelope, err := core.fetchSchemaEnvelope("aws-iam-purger", "purge"); err != nil {
		t.Fatalf("purge schema endpoint: %v", err)
	} else if envelope["provider"] != "aws-iam-purger" || envelope["category"] != "purge" {
		t.Fatalf("unexpected purge schema envelope: %v", envelope)
	}

	staticAccess := map[string]any{"accessKeyId": purgeAWSAccessKeyID, "secretAccessKey": purgeAWSSecretAccessKey}
	imdsAccess := map[string]any{"authMethod": "imds", "accessKeyId": purgeAWSAccessKeyID, "secretAccessKey": purgeAWSSecretAccessKey}
	defaultChainAccess := map[string]any{"authMethod": "", "accessKeyId": "", "secretAccessKey": ""}

	request := func(name, provider string, accessConfig map[string]any, providerConfig map[string]any, expiredDays float64) purgeWorkflowRequest {
		return purgeWorkflowRequest{
			name:           name,
			provider:       provider,
			accessProvider: "aws",
			accessConfig:   accessConfig,
			providerConfig: providerConfig,
			expiredDays:    expiredDays,
		}
	}
	acmConfig := map[string]any{"region": purgeAWSRegion}
	iamConfig := map[string]any{"region": purgeAWSRegion}
	iamScopedConfig := map[string]any{"region": purgeAWSRegion, "certificatePath": "/team-a/"}

	run := func(req purgeWorkflowRequest) purgeWorkflowResult {
		return core.runPurgeWorkflow(ctx, req, purgeAWSSecretAccessKey, purgeAWSIMDSSecretKey)
	}
	assertDeletes := func(t *testing.T, got, want []string) {
		t.Helper()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("deleted certificates %v, want %v", got, want)
		}
	}
	assertRemaining := func(t *testing.T, got, want []string) {
		t.Helper()
		sorted := append([]string{}, got...)
		sort.Strings(sorted)
		expected := append([]string{}, want...)
		sort.Strings(expected)
		if strings.Join(sorted, ",") != strings.Join(expected, ",") {
			t.Fatalf("remaining certificates %v, want %v", sorted, expected)
		}
	}
	assertSignedBy := func(t *testing.T, credentials []string, keyID string) {
		t.Helper()
		if len(credentials) == 0 {
			t.Fatalf("no signed requests recorded, want credential %s", keyID)
		}
		for _, credential := range credentials {
			if credential != keyID {
				t.Fatalf("requests signed by %q, want %q", credential, keyID)
			}
		}
	}
	t.Run("acm_success_threshold_static_auth", func(t *testing.T) {
		fixture.acm.reset()
		fixture.acmSeed("arn:aws:acm:old-30d", 30)
		fixture.acmSeed("arn:aws:acm:old-9d", 9)
		fixture.acmSeed("arn:aws:acm:new-2d", 2)
		fixture.acmSeed("arn:aws:acm:future", -30)
		credentialsBefore := len(fixture.acmCredentials)

		result := run(request("Purge aws acm success", "aws-acm-purger", staticAccess, acmConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, fixture.acm.deletes, []string{"arn:aws:acm:old-30d", "arn:aws:acm:old-9d"})
		assertRemaining(t, fixture.acm.remaining(), []string{"arn:aws:acm:new-2d", "arn:aws:acm:future"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		assertSignedBy(t, fixture.acmCredentials[credentialsBefore:], purgeAWSAccessKeyID)
	})

	t.Run("acm_acme_origin_and_missing_notafter_excluded", func(t *testing.T) {
		fixture.acm.reset()
		fixture.acmSeedExotic("arn:aws:acm:acme-old", 30, "ACME", false)
		fixture.acmSeedExotic("arn:aws:acm:no-expiry-old", 30, "", true)
		fixture.acmSeedExotic("arn:aws:acm:imported-old", 30, "IMPORTED", false)

		result := run(request("Purge aws acm exclusions", "aws-acm-purger", staticAccess, acmConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, fixture.acm.deletes, []string{"arn:aws:acm:imported-old"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 1 {
			t.Fatalf("purge completed log count=%d found=%t, want 1", count, ok)
		}
		if !contains(fixture.acm.remaining(), "arn:aws:acm:acme-old") || !contains(fixture.acm.remaining(), "arn:aws:acm:no-expiry-old") {
			t.Fatalf("excluded certificates were purged: %v", fixture.acm.remaining())
		}
	})

	t.Run("acm_multipage_list", func(t *testing.T) {
		fixture.acm.reset()
		fixture.mu.Lock()
		fixture.acm.pageSize = 2
		fixture.mu.Unlock()
		defer func() {
			fixture.mu.Lock()
			fixture.acm.pageSize = 0
			fixture.mu.Unlock()
		}()
		fixture.acmSeed("arn:aws:acm:page-fresh-1", 2)
		fixture.acmSeed("arn:aws:acm:page-fresh-2", 2)
		fixture.acmSeed("arn:aws:acm:page-fresh-3", 2)
		fixture.acmSeed("arn:aws:acm:page-fresh-4", 2)
		fixture.acmSeed("arn:aws:acm:page-old", 30)
		listsBefore := fixture.acm.lists
		fixture.mu.Lock()
		total := len(fixture.acm.certs)
		fixture.mu.Unlock()
		wantLists := (total + 1) / 2

		result := run(request("Purge aws acm multipage", "aws-acm-purger", staticAccess, acmConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if fixture.acm.lists-listsBefore != wantLists {
			t.Fatalf("list calls %d, want %d (2 per page)", fixture.acm.lists-listsBefore, wantLists)
		}
		if !contains(fixture.acm.deletes, "arn:aws:acm:page-old") {
			t.Fatalf("multipage deletes %v, want page-old present", fixture.acm.deletes)
		}
	})

	t.Run("acm_delete_notfound_and_inuse_skip", func(t *testing.T) {
		fixture.acm.reset()
		fixture.acmSeed("arn:aws:acm:vanish", 15)
		fixture.acmSeed("arn:aws:acm:inuse", 15)
		fixture.mu.Lock()
		fixture.acm.vanishARN = "arn:aws:acm:vanish"
		fixture.acm.inUseARN = "arn:aws:acm:inuse"
		fixture.mu.Unlock()

		result := run(request("Purge aws acm skips", "aws-acm-purger", staticAccess, acmConfig, 7))
		fixture.mu.Lock()
		fixture.acm.vanishARN = ""
		fixture.acm.inUseARN = ""
		fixture.mu.Unlock()
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("skip log count=%d found=%t, want 0", count, ok)
		}
		if !contains(fixture.acm.remaining(), "arn:aws:acm:vanish") || !contains(fixture.acm.remaining(), "arn:aws:acm:inuse") {
			t.Fatalf("skipped certificates were removed: %v", fixture.acm.remaining())
		}
	})

	t.Run("acm_delete_failure_and_repair", func(t *testing.T) {
		fixture.acm.reset()
		fixture.acmSeed("arn:aws:acm:pf-a", 20)
		fixture.acmSeed("arn:aws:acm:pf-b", 20)
		fixture.mu.Lock()
		fixture.acm.denyARN = "arn:aws:acm:pf-b"
		fixture.mu.Unlock()

		req := request("Purge aws acm delete failure", "aws-acm-purger", staticAccess, acmConfig, 7)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}

		fixture.mu.Lock()
		fixture.acm.denyARN = ""
		fixture.mu.Unlock()
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		if strings.Count(strings.Join(fixture.acm.deletes, ","), "arn:aws:acm:pf-a") != 1 || !contains(fixture.acm.deletes, "arn:aws:acm:pf-b") {
			t.Fatalf("repair deleted %v", fixture.acm.deletes)
		}
	})

	t.Run("acm_list_failure", func(t *testing.T) {
		fixture.acm.reset()
		fixture.acmSeed("arn:aws:acm:list-fail", 30)
		deletesBefore := len(fixture.acm.deletes)
		fixture.mu.Lock()
		fixture.acm.listFail = true
		fixture.mu.Unlock()
		result := run(request("Purge aws acm list failure", "aws-acm-purger", staticAccess, acmConfig, 7))
		fixture.mu.Lock()
		fixture.acm.listFail = false
		fixture.mu.Unlock()
		if result.status != "failed" {
			t.Fatalf("list failure status %s, want failed", result.status)
		}
		if len(fixture.acm.deletes) != deletesBefore {
			t.Fatalf("list failure still deleted %v", fixture.acm.deletes[deletesBefore:])
		}
	})

	t.Run("acm_auth_failure", func(t *testing.T) {
		fixture.acm.reset()
		fixture.acmSeed("arn:aws:acm:auth", 30)
		deletesBefore := len(fixture.acm.deletes)
		fixture.setMode("auth-failure")
		result := run(request("Purge aws acm auth failure", "aws-acm-purger", staticAccess, acmConfig, 7))
		fixture.setMode("")
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
		if len(fixture.acm.deletes) != deletesBefore {
			t.Fatalf("auth failure still deleted %v", fixture.acm.deletes[deletesBefore:])
		}
	})

	t.Run("acm_imds_auth_method_ignores_static_keys", func(t *testing.T) {
		fixture.acm.reset()
		fixture.acmSeed("arn:aws:acm:imds", 30)
		credentialsBefore := len(fixture.acmCredentials)
		credsBefore := fixture.imdsCredsFetch

		result := run(request("Purge aws acm imds", "aws-acm-purger", imdsAccess, acmConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertSignedBy(t, fixture.acmCredentials[credentialsBefore:], purgeAWSIMDSAccessKeyID)
		if fixture.imdsCredsFetch == credsBefore {
			t.Fatal("imds credential endpoint was never contacted")
		}
		if !contains(fixture.acm.deletes, "arn:aws:acm:imds") {
			t.Fatalf("imds-authenticated purge deleted %v", fixture.acm.deletes)
		}
	})

	t.Run("acm_default_chain_resolves_imds", func(t *testing.T) {
		fixture.acm.reset()
		fixture.acmSeed("arn:aws:acm:default-chain", 30)
		credentialsBefore := len(fixture.acmCredentials)

		result := run(request("Purge aws acm default chain", "aws-acm-purger", defaultChainAccess, acmConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertSignedBy(t, fixture.acmCredentials[credentialsBefore:], purgeAWSIMDSAccessKeyID)
		if !contains(fixture.acm.deletes, "arn:aws:acm:default-chain") {
			t.Fatalf("default-chain purge deleted %v", fixture.acm.deletes)
		}
	})

	t.Run("iam_success_path_scope_and_threshold", func(t *testing.T) {
		fixture.iam.reset()
		fixture.iamSeed("iam-root-old", "/", 30)
		fixture.iamSeed("iam-team-a-old", "/team-a/", 30)
		fixture.iamSeed("iam-team-a-new", "/team-a/", 2)
		fixture.iamSeed("iam-team-b-old", "/team-b/", 30)
		prefixesBefore := len(fixture.iam.pathPrefixes)

		result := run(request("Purge aws iam path scope", "aws-iam-purger", staticAccess, iamScopedConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, fixture.iam.deletes, []string{"iam-team-a-old"})
		assertRemaining(t, fixture.iam.remaining(), []string{"iam-root-old", "iam-team-a-new", "iam-team-b-old"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 1 {
			t.Fatalf("purge completed log count=%d found=%t, want 1", count, ok)
		}
		for _, prefix := range fixture.iam.pathPrefixes[prefixesBefore:] {
			if prefix != "/team-a/" {
				t.Fatalf("iam list PathPrefix %q, want /team-a/", prefix)
			}
		}
		assertSignedBy(t, fixture.iamCredentials, purgeAWSAccessKeyID)
	})

	t.Run("iam_default_path_lists_all", func(t *testing.T) {
		fixture.iam.reset()
		fixture.iamSeed("iam-default-root-old", "/", 30)
		fixture.iamSeed("iam-default-nested-old", "/nested/path/", 30)
		fixture.iamSeed("iam-default-new", "/", 2)
		prefixesBefore := len(fixture.iam.pathPrefixes)

		result := run(request("Purge aws iam default path", "aws-iam-purger", staticAccess, iamConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, fixture.iam.deletes, []string{"iam-default-root-old", "iam-default-nested-old"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		for _, prefix := range fixture.iam.pathPrefixes[prefixesBefore:] {
			if prefix != "/" {
				t.Fatalf("default certificatePath produced PathPrefix %q, want /", prefix)
			}
		}
	})

	t.Run("iam_missing_expiration_skipped", func(t *testing.T) {
		fixture.iam.reset()
		fixture.mu.Lock()
		fixture.iam.seed("iam-no-expiry", "/", 30, true)
		fixture.mu.Unlock()

		result := run(request("Purge aws iam no expiry", "aws-iam-purger", staticAccess, iamConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("no-expiry log count=%d found=%t, want 0", count, ok)
		}
		if !contains(fixture.iam.remaining(), "iam-no-expiry") {
			t.Fatal("certificate without expiration was purged")
		}
	})

	t.Run("iam_multipage_list", func(t *testing.T) {
		fixture.iam.reset()
		fixture.mu.Lock()
		fixture.iam.pageSize = 2
		fixture.mu.Unlock()
		defer func() {
			fixture.mu.Lock()
			fixture.iam.pageSize = 0
			fixture.mu.Unlock()
		}()
		for i := 0; i < 5; i++ {
			fixture.iamSeed(fmt.Sprintf("iam-page-fresh-%d", i), "/", 2)
		}
		fixture.iamSeed("iam-page-old", "/", 30)
		listsBefore := fixture.iam.lists
		fixture.mu.Lock()
		total := len(fixture.iam.certs)
		fixture.mu.Unlock()
		wantLists := (total + 1) / 2

		result := run(request("Purge aws iam multipage", "aws-iam-purger", staticAccess, iamConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if fixture.iam.lists-listsBefore != wantLists {
			t.Fatalf("list calls %d, want %d (2 per page)", fixture.iam.lists-listsBefore, wantLists)
		}
		if !contains(fixture.iam.deletes, "iam-page-old") {
			t.Fatalf("multipage deletes %v, want iam-page-old present", fixture.iam.deletes)
		}
	})

	t.Run("iam_delete_notfound_and_conflict_skip", func(t *testing.T) {
		fixture.iam.reset()
		fixture.iamSeed("iam-vanish", "/", 15)
		fixture.iamSeed("iam-conflict", "/", 15)
		fixture.mu.Lock()
		fixture.iam.vanishName = "iam-vanish"
		fixture.iam.conflictName = "iam-conflict"
		fixture.mu.Unlock()

		result := run(request("Purge aws iam skips", "aws-iam-purger", staticAccess, iamConfig, 7))
		fixture.mu.Lock()
		fixture.iam.vanishName = ""
		fixture.iam.conflictName = ""
		fixture.mu.Unlock()
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("skip log count=%d found=%t, want 0", count, ok)
		}
		if !contains(fixture.iam.remaining(), "iam-vanish") || !contains(fixture.iam.remaining(), "iam-conflict") {
			t.Fatalf("skipped certificates were removed: %v", fixture.iam.remaining())
		}
	})

	t.Run("iam_delete_failure_and_repair", func(t *testing.T) {
		fixture.iam.reset()
		fixture.iamSeed("iam-pf-a", "/", 20)
		fixture.iamSeed("iam-pf-b", "/", 20)
		fixture.mu.Lock()
		fixture.iam.denyName = "iam-pf-b"
		fixture.mu.Unlock()

		req := request("Purge aws iam delete failure", "aws-iam-purger", staticAccess, iamConfig, 7)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}

		fixture.mu.Lock()
		fixture.iam.denyName = ""
		fixture.mu.Unlock()
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		if strings.Count(strings.Join(fixture.iam.deletes, ","), "iam-pf-a") != 1 || !contains(fixture.iam.deletes, "iam-pf-b") {
			t.Fatalf("repair deleted %v", fixture.iam.deletes)
		}
	})

	t.Run("iam_list_failure", func(t *testing.T) {
		fixture.iam.reset()
		fixture.iamSeed("iam-list-fail", "/", 30)
		deletesBefore := len(fixture.iam.deletes)
		fixture.mu.Lock()
		fixture.iam.listFail = true
		fixture.mu.Unlock()
		result := run(request("Purge aws iam list failure", "aws-iam-purger", staticAccess, iamConfig, 7))
		fixture.mu.Lock()
		fixture.iam.listFail = false
		fixture.mu.Unlock()
		if result.status != "failed" {
			t.Fatalf("list failure status %s, want failed", result.status)
		}
		if len(fixture.iam.deletes) != deletesBefore {
			t.Fatalf("list failure still deleted %v", fixture.iam.deletes[deletesBefore:])
		}
	})

	t.Run("iam_auth_failure", func(t *testing.T) {
		fixture.iam.reset()
		fixture.iamSeed("iam-auth", "/", 30)
		deletesBefore := len(fixture.iam.deletes)
		fixture.setMode("auth-failure")
		result := run(request("Purge aws iam auth failure", "aws-iam-purger", staticAccess, iamConfig, 7))
		fixture.setMode("")
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
		if len(fixture.iam.deletes) != deletesBefore {
			t.Fatalf("auth failure still deleted %v", fixture.iam.deletes[deletesBefore:])
		}
	})
}

func runPurgeAWSInDocker(t *testing.T) {
	t.Helper()
	runPurgeFamilyInDocker(t, purgeAWSPlugins, "purge-aws.test", "^TestPurgeWorkflowAWS$")
}
