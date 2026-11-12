package integration

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

type b6Reply func(int, any)

type b6UploadedCertificate struct {
	name string
	pem  b6Certificate
	leaf *x509.Certificate
}

func (f *b6CloudTarget) upload(name string, pem b6Certificate) (int, bool) {
	leaf, err := xcert.ParseCertificateFromPEM(pem.Certificate)
	if err != nil || pem.PrivateKey == "" || name == "" {
		return 0, false
	}
	f.uploads = append(f.uploads, b6UploadedCertificate{name, pem, leaf})
	return len(f.uploads), true
}

func (f *b6CloudTarget) bind(id int, reply b6Reply) bool {
	if id <= 0 || id > len(f.uploads) {
		reply(http.StatusBadRequest, map[string]any{"message": "binding references an unknown certificate"})
		return false
	}
	if f.mode == "write-failure" {
		reply(http.StatusServiceUnavailable, map[string]any{"message": "fixture blocked binding"})
		return false
	}
	f.bound = id - 1
	f.current = f.uploads[f.bound].pem
	f.writes++
	return true
}

func (f *b6CloudTarget) boundName() string {
	if f.bound >= 0 {
		return f.uploads[f.bound].name
	}
	return ""
}

type b6EdgePaths struct {
	list, detail, create, getBinding, setBinding string
}

var b6CTCCEdgePaths = map[string]b6EdgePaths{
	b6AOHost: {
		"GET /ctapi/v1/accessone/cert/list", "GET /ctapi/v1/accessone/cert/query", "POST /ctapi/v1/accessone/cert/create",
		"POST /ctapi/v1/accessone/domain/config", "POST /ctapi/v1/scdn/domain/modify_config",
	},
	b6CTCDNHost: {
		"GET /v1/cert/query-cert-list", "GET /v1/cert/query-cert-detail", "POST /v1/cert/creat-cert",
		"GET /v1/domain/query-domain-detail", "POST /v1/domain/update-domain",
	},
	b6ICDNHost: {
		"GET /v1/cert/query-cert-list", "GET /v1/cert/query-cert-detail", "POST /v1/cert/creat-cert",
		"GET /v1/domain/query-domain-detail", "POST /v1/domain/update-domain",
	},
	b6LVDNHost: {
		"GET /cert/query-cert-list", "GET /cert/query-cert-detail", "POST /cert/creat-cert",
		"POST /live/domain/query-domain-detail", "POST /live/domain/update-domain",
	},
}

func (f *b6CloudTarget) serveCTCCEdge(r *http.Request, reply b6Reply) {
	paths := b6CTCCEdgePaths[r.Host]
	lvdn := r.Host == b6LVDNHost
	ao := r.Host == b6AOHost
	ok := func(payload map[string]any) {
		if lvdn {
			payload["code"] = 100000
			reply(http.StatusOK, payload)
		} else {
			reply(http.StatusOK, map[string]any{"statusCode": 100000, "returnObj": payload})
		}
	}
	switch r.Method + " " + r.URL.Path {
	case paths.list:
		rows := make([]map[string]any, 0, len(f.uploads))
		for i, item := range f.uploads {
			rows = append(rows, map[string]any{
				"id": i + 1, "name": item.name, "cn": item.leaf.Subject.CommonName, "sans": item.leaf.DNSNames,
				"expires": item.leaf.NotAfter.Unix(), "issue": item.leaf.NotBefore.Unix(),
			})
		}
		ok(map[string]any{"result": rows})
	case paths.detail:
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil || id <= 0 || id > len(f.uploads) {
			reply(http.StatusNotFound, map[string]any{"message": "unknown edge certificate"})
			return
		}
		item := f.uploads[id-1]
		ok(map[string]any{"result": map[string]any{"id": id, "name": item.name, "certs": item.pem.Certificate}})
	case paths.create:
		var input struct {
			Name string `json:"name"`
			Cert string `json:"certs"`
			Key  string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			reply(http.StatusBadRequest, map[string]any{"message": "invalid edge upload"})
			return
		}
		id, uploaded := f.upload(input.Name, b6Certificate{Certificate: input.Cert, PrivateKey: input.Key})
		if !uploaded {
			reply(http.StatusBadRequest, map[string]any{"message": "invalid edge certificate"})
			return
		}
		ok(map[string]any{"id": id})
	case paths.getBinding:
		domain := r.URL.Query().Get("domain")
		if r.Method == http.MethodPost {
			var input struct {
				Domain string `json:"domain"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err == nil {
				domain = input.Domain
			}
		}
		if domain != b6Domain {
			reply(http.StatusBadRequest, map[string]any{"message": "unexpected edge domain"})
			return
		}
		state := map[string]any{"domain": domain, "cert_name": f.boundName(), "https_status": "off", "https_switch": 0}
		if f.bound >= 0 {
			state["https_status"] = "on"
			state["https_switch"] = 1
		}
		if ao {
			state["product_code"] = "020"
			state["origin"] = []map[string]any{{"origin": "origin.fixture.example", "role": "master", "weight": 1}}
		}
		ok(state)
	case paths.setBinding:
		var input struct {
			Domain      string `json:"domain"`
			ProductCode string `json:"product_code"`
			HTTPS       string `json:"https_status"`
			HTTPSSwitch int    `json:"https_switch"`
			Name        string `json:"cert_name"`
			Origin      []struct {
				Name   string `json:"origin"`
				Role   string `json:"role"`
				Weight string `json:"weight"`
			} `json:"origin"`
		}
		err := json.NewDecoder(r.Body).Decode(&input)
		if err != nil || input.Domain != b6Domain ||
			(lvdn && (input.HTTPSSwitch != 1 || input.ProductCode != "005")) ||
			(!lvdn && input.HTTPS != "on") ||
			(ao && (input.ProductCode != "020" || len(input.Origin) != 1 || input.Origin[0].Name != "origin.fixture.example" || input.Origin[0].Role != "master" || input.Origin[0].Weight != "1")) {
			reply(http.StatusBadRequest, map[string]any{"message": "unexpected edge binding"})
			return
		}
		id := 0
		for i, item := range f.uploads {
			if item.name == input.Name {
				id = i + 1
			}
		}
		if f.bind(id, reply) {
			ok(map[string]any{})
		}
	default:
		reply(http.StatusNotFound, map[string]any{"message": "unexpected edge request: " + r.Method + " " + r.URL.Path})
	}
}

func (f *b6CloudTarget) serveCTCCELB(r *http.Request, reply b6Reply) {
	ok := func(payload any) { reply(http.StatusOK, map[string]any{"statusCode": 200, "returnObj": payload}) }
	switch r.Method + " " + r.URL.Path {
	case "GET /v4/elb/list-certificate":
		if r.URL.Query().Get("regionID") != b6Region {
			reply(http.StatusBadRequest, map[string]any{"message": "unexpected ELB region"})
			return
		}
		rows := make([]map[string]any, 0, len(f.uploads))
		for i, item := range f.uploads {
			rows = append(rows, map[string]any{"id": strconv.Itoa(i + 1), "name": item.name, "certificate": item.pem.Certificate})
		}
		ok(rows)
	case "POST /v4/elb/create-certificate":
		var input struct {
			b6Certificate
			Region string `json:"regionID"`
			Token  string `json:"clientToken"`
			Type   string `json:"type"`
			Name   string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Region != b6Region || len(input.Token) != 32 || input.Type != "Server" {
			reply(http.StatusBadRequest, map[string]any{"message": "invalid ELB upload"})
			return
		}
		id, uploaded := f.upload(input.Name, input.b6Certificate)
		if !uploaded {
			reply(http.StatusBadRequest, map[string]any{"message": "invalid ELB certificate"})
			return
		}
		ok(map[string]any{"id": strconv.Itoa(id)})
	case "POST /v4/elb/update-listener":
		var input struct {
			Region      string `json:"regionID"`
			Listener    string `json:"listenerID"`
			Certificate string `json:"certificateID"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Region != b6Region || input.Listener != b6Listener {
			reply(http.StatusBadRequest, map[string]any{"message": "unexpected ELB listener"})
			return
		}
		id, _ := strconv.Atoi(input.Certificate)
		if f.bind(id, reply) {
			ok([]any{})
		}
	default:
		reply(http.StatusNotFound, map[string]any{"message": "unexpected ELB request"})
	}
}

func (f *b6CloudTarget) serveCTCCFaaS(r *http.Request, reply b6Reply) {
	if r.URL.Path != "/openapi/v1/domains/customdomains/"+b6Domain || r.Header.Get("RegionId") != b6Region {
		reply(http.StatusBadRequest, map[string]any{"message": "unexpected FaaS domain or region"})
		return
	}
	ok := func(payload any) { reply(http.StatusOK, map[string]any{"statusCode": 0, "returnObj": payload}) }
	switch r.Method {
	case http.MethodGet:
		state := map[string]any{"domainName": b6Domain, "protocol": "HTTP", "authConfig": map[string]any{"authType": "NONE"}}
		if f.writes > 0 {
			state["protocol"] = "HTTP,HTTPS"
			state["certConfig"] = f.current
		}
		ok(state)
	case http.MethodPut:
		var input struct {
			Domain string `json:"domainName"`
			Proto  string `json:"protocol"`
			Auth   struct {
				Type string `json:"authType"`
			} `json:"authConfig"`
			Certificate b6Certificate `json:"certConfig"`
		}
		err := json.NewDecoder(r.Body).Decode(&input)
		if err != nil || input.Domain != b6Domain || input.Proto != "HTTP,HTTPS" || input.Auth.Type != "NONE" ||
			input.Certificate.Certificate == "" || input.Certificate.PrivateKey == "" {
			reply(http.StatusBadRequest, map[string]any{"message": "unexpected FaaS certificate update"})
			return
		}
		if f.mode == "write-failure" {
			reply(http.StatusServiceUnavailable, map[string]any{"message": "fixture blocked FaaS binding"})
			return
		}
		f.current = input.Certificate
		f.writes++
		ok(map[string]any{})
	default:
		reply(http.StatusNotFound, map[string]any{"message": "unexpected FaaS request"})
	}
}

func (f *b6CloudTarget) serveCMCCVLB(r *http.Request, reply b6Reply) {
	const certificationPath = "/lb-console/acl/v3/certification"
	path := strings.TrimPrefix(r.URL.Path, "/api/openapi-vlb")
	ok := func(payload any) { reply(http.StatusOK, map[string]any{"body": payload}) }
	switch r.Method + " " + path {
	case "GET " + certificationPath:
		rows := make([]map[string]any, 0, len(f.uploads))
		for i, item := range f.uploads {
			rows = append(rows, map[string]any{
				"id": strconv.Itoa(i + 1), "name": item.name,
				"expirationTime": item.leaf.NotAfter.In(time.FixedZone("CST", 8*60*60)).Format(time.DateTime),
			})
		}
		ok(map[string]any{"content": rows})
	case "POST " + certificationPath:
		var input struct {
			Name string `json:"name"`
			Type string `json:"type"`
			Cert string `json:"publicKey"`
			Key  string `json:"privateKey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Type != "SERVER" {
			reply(http.StatusBadRequest, map[string]any{"message": "invalid VLB upload"})
			return
		}
		id, uploaded := f.upload(input.Name, b6Certificate{Certificate: input.Cert, PrivateKey: input.Key})
		if !uploaded {
			reply(http.StatusBadRequest, map[string]any{"message": "invalid VLB certificate"})
			return
		}
		ok(strconv.Itoa(id))
	case "GET /lb-console/protocol/v3/listener/" + b6LB + "/listeners/https":
		query := r.URL.Query()
		if query.Get("page") != "1" || query.Get("pageSize") != "10" {
			reply(http.StatusBadRequest, map[string]any{"message": "invalid VLB listener pagination"})
			return
		}
		row := map[string]any{"id": b6Listener, "protocol": "HTTPS"}
		if f.bound >= 0 {
			row["defaultTlsContainerId"] = strconv.Itoa(f.bound + 1)
		}
		ok(map[string]any{"content": []map[string]any{row}})
	case "PUT /lb-console/acl/v3/listener":
		var input struct {
			ID          string `json:"id"`
			Certificate string `json:"defaultTlsContainerId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.ID != b6Listener {
			reply(http.StatusBadRequest, map[string]any{"message": "unexpected VLB listener"})
			return
		}
		id, _ := strconv.Atoi(input.Certificate)
		if f.bind(id, reply) {
			reply(http.StatusOK, map[string]any{})
		}
	default:
		if r.Method == http.MethodGet && strings.HasPrefix(path, certificationPath+"/") {
			id, err := strconv.Atoi(strings.TrimPrefix(path, certificationPath+"/"))
			if err == nil && id > 0 && id <= len(f.uploads) {
				ok(map[string]any{"publicKey": f.uploads[id-1].pem.Certificate})
				return
			}
		}
		reply(http.StatusNotFound, map[string]any{"message": fmt.Sprintf("unexpected VLB request: %s %s", r.Method, r.URL.Path)})
	}
}
