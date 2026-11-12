package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (f *b7HuaweiTarget) serveProduct(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	send := func(body any) { _ = json.NewEncoder(w).Encode(body) }
	if f.rejectAuth || !strings.Contains(r.Header.Get("Authorization"), "Access="+b7Key+",") {
		w.WriteHeader(http.StatusUnauthorized)
		send(map[string]string{"error_code": "B7_DENIED", "error_msg": "fixture credentials rejected"})
		return
	}
	if r.Host == b7WAFHost && f.serveWAFStore(r, send) {
		return
	}
	if f.bindProduct(r) {
		f.bindings++
		send(struct{}{})
		return
	}
	switch {
	case r.Host == b7AADHost && r.Method == http.MethodGet && r.URL.Path == "/v2/aad/instances/fixture-instance/domains":
		send(map[string]any{"domains": []map[string]string{
			{"domain_id": "fixture-domain-id", "domain_name": b7Domain, "domain_status": "0"},
			{"domain_id": "other-domain-id", "domain_name": "other.example", "domain_status": "0"},
		}})
	case r.Host == b7APIGHost && r.Method == http.MethodGet && r.URL.Path == "/v2/"+b7IAMProject+"/apigw/certificates/fixture-cert":
		send(map[string]string{"type": "instance", "instance_id": "fixture-instance"})
	case r.Host == b7VODHost && r.Method == http.MethodGet && r.URL.Path == "/v1.0/"+b7IAMProject+"/asset/domain/https" &&
		r.URL.Query().Get("domain") == b7Domain:
		send(map[string]any{"cert_id": f.boundID, "http2": 1, "force_redirect_https": 1})
	case r.Host == b7WAFHost && r.Method == http.MethodGet && r.URL.Path == "/v1/"+b7IAMProject+"/waf/instance" &&
		r.URL.Query().Get("hostname") == b7Domain:
		send(map[string]any{"items": []map[string]string{{"id": "fixture-host-id", "hostname": b7Domain}}})
	default:
		http.Error(w, fmt.Sprintf("unexpected Huawei deployment request %s %s", r.Method, r.URL.RequestURI()), http.StatusBadRequest)
	}
}

func (f *b7HuaweiTarget) bindProduct(r *http.Request) bool {
	var id, cert, key string
	decode := func(out any) bool { return json.NewDecoder(r.Body).Decode(out) == nil }
	switch {
	case r.Host == b7APIGHost && r.Method == http.MethodPut && r.URL.Path == "/v2/"+b7IAMProject+"/apigw/certificates/fixture-cert":
		var body struct {
			Cert       string `json:"cert_content"`
			Key        string `json:"private_key"`
			Type       string `json:"type"`
			InstanceID string `json:"instance_id"`
		}
		if !decode(&body) || body.Type != "instance" || body.InstanceID != "fixture-instance" {
			return false
		}
		cert, key = body.Cert, body.Key
	case r.Host == b7CDNHost && r.Method == http.MethodPut && r.URL.Path == "/v1.0/cdn/domains/config-https-info":
		var body struct {
			HTTPS struct {
				ID     string `json:"scm_certificate_id"`
				Domain string `json:"domain_name"`
				Switch int    `json:"https_switch"`
				Type   int    `json:"certificate_type"`
			} `json:"https"`
		}
		if !decode(&body) || body.HTTPS.Domain != b7Domain || body.HTTPS.Switch != 1 || body.HTTPS.Type != 2 {
			return false
		}
		id = body.HTTPS.ID
	case r.Host == b7LiveHost && r.Method == http.MethodPut && r.URL.Path == "/v1/"+b7IAMProject+"/guard/https-cert" &&
		r.URL.Query().Get("domain") == b7Domain:
		var body struct {
			TLS struct {
				ID     string `json:"cert_id"`
				Source string `json:"source"`
			} `json:"tls_certificate"`
		}
		if !decode(&body) || body.TLS.Source != "scm" {
			return false
		}
		id = body.TLS.ID
	case r.Host == b7VODHost && r.Method == http.MethodPut && r.URL.Path == "/v1.0/"+b7IAMProject+"/asset/domain/https":
		var body struct {
			ID       string `json:"cert_id"`
			Source   string `json:"source"`
			Domain   string `json:"domain"`
			HTTPS    int    `json:"https_status"`
			HTTP2    int    `json:"http2"`
			Redirect int    `json:"force_redirect_https"`
		}
		if !decode(&body) || body.Source != "scm" || body.Domain != b7Domain || body.HTTPS != 1 || body.HTTP2 != 1 || body.Redirect != 1 {
			return false
		}
		id = body.ID
	case r.Host == b7ELBHost && r.Method == http.MethodPut && r.URL.Path == "/v3/"+b7IAMProject+"/elb/certificates/fixture-cert":
		var body struct {
			Certificate struct {
				Cert string `json:"certificate"`
				Key  string `json:"private_key"`
			} `json:"certificate"`
		}
		if !decode(&body) {
			return false
		}
		cert, key = body.Certificate.Cert, body.Certificate.Key
	case r.Host == b7AADHost && r.Method == http.MethodPost && r.URL.Path == "/v1/"+b7IAMProject+"/aad/external/domains/certificate":
		var body struct {
			DomainID string `json:"domain_id"`
			Cert     string `json:"cert_file"`
			Key      string `json:"cert_key_file"`
		}
		if !decode(&body) || body.DomainID != "fixture-domain-id" || body.Cert == "" || body.Key == "" {
			return false
		}
		cert, key = body.Cert, body.Key
	case r.Host == b7WAFHost && r.Method == http.MethodPatch && r.URL.Path == "/v1/"+b7IAMProject+"/waf/instance/fixture-host-id":
		var body struct {
			ID   string `json:"certificateid"`
			Name string `json:"certificatename"`
		}
		if !decode(&body) || len(f.certs) == 0 || body.Name != f.certs[len(f.certs)-1].Name {
			return false
		}
		id = body.ID
	default:
		return false
	}
	if id != "" && id == strconv.Itoa(len(f.certs)) {
		f.boundID = id
		return true
	}
	if cert == "" || key == "" {
		return false
	}
	f.replacement = b7Certificate{Certificate: cert, PrivateKey: key}
	return true
}

func (f *b7HuaweiTarget) serveWAFStore(r *http.Request, send func(any)) bool {
	path := "/v1/" + b7IAMProject + "/waf/certificate"
	switch {
	case r.URL.Path == path && r.Method == http.MethodGet:
		items := make([]map[string]string, 0, len(f.certs))
		for i, c := range f.certs {
			items = append(items, map[string]string{"id": strconv.Itoa(i + 1), "name": c.Name})
		}
		send(map[string]any{"items": items, "total": len(items)})
	case r.URL.Path == path && r.Method == http.MethodPost:
		var body struct {
			Name string `json:"name"`
			Cert string `json:"content"`
			Key  string `json:"key"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name == "" || body.Cert == "" || body.Key == "" {
			return false
		}
		f.certs = append(f.certs, b7Certificate{Name: body.Name, Certificate: body.Cert, PrivateKey: body.Key})
		send(map[string]string{"id": strconv.Itoa(len(f.certs))})
	case strings.HasPrefix(r.URL.Path, path+"/") && r.Method == http.MethodGet:
		id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, path+"/"))
		if err != nil || id < 1 || id > len(f.certs) {
			return false
		}
		f.exports++
		send(map[string]string{"name": f.certs[id-1].Name, "content": f.certs[id-1].Certificate})
	default:
		return false
	}
	return true
}
