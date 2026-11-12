package rainyun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

type certificateInput struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type certificateRecord struct {
	ID         int64  `json:"ID"`
	Domain     string `json:"Domain"`
	StartDate  int64  `json:"StartDate"`
	ExpireDate int64  `json:"ExpDate"`
}

type certificateList struct {
	Records []certificateRecord `json:"Records"`
}

type certificateDetail struct {
	Cert string `json:"Cert"`
}

type certificateFilter struct {
	Domain string `json:"Domain"`
}

type certificateListOptions struct {
	Filters certificateFilter `json:"columnFilters"`
	Page    int               `json:"page"`
	PerPage int               `json:"perPage"`
}

func (c *Client) Upload(ctx context.Context, certPEM, keyPEM string) (int64, error) {
	id, found, err := c.findCertificate(ctx, certPEM)
	if err != nil || found {
		return id, err
	}

	req := c.http.R().SetContext(ctx).SetBody(certificateInput{Cert: certPEM, Key: keyPEM})
	if _, err := send[any](req, http.MethodPost, "/product/sslcenter/"); err != nil {
		return 0, fmt.Errorf("failed to execute sdk request 'sslcenter.Create': %w", err)
	}

	id, found, err = c.findCertificate(ctx, certPEM)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, fmt.Errorf("could not find ssl certificate, may be upload failed")
	}
	return id, nil
}

func (c *Client) Replace(ctx context.Context, id int64, certPEM, keyPEM string) error {
	req := c.http.R().SetContext(ctx).SetBody(certificateInput{Cert: certPEM, Key: keyPEM})
	_, err := send[any](req, http.MethodPut, fmt.Sprintf("/product/sslcenter/%d", id))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'sslcenter.Update': %w", err)
	}
	return nil
}

func (c *Client) findCertificate(ctx context.Context, certPEM string) (int64, bool, error) {
	cert, err := xcert.ParseCertificateFromPEM(certPEM)
	if err != nil {
		return 0, false, err
	}

	domains := strings.Join(cert.DNSNames, ", ")
	const perPage = 100
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return 0, false, err
		}
		options, _ := json.Marshal(certificateListOptions{
			Filters: certificateFilter{Domain: cert.Subject.CommonName},
			Page:    page,
			PerPage: perPage,
		})
		req := c.http.R().SetContext(ctx).SetQueryParam("options", string(options))
		list, err := send[certificateList](req, http.MethodGet, "/product/sslcenter")
		if err != nil {
			return 0, false, fmt.Errorf("failed to execute sdk request 'sslcenter.List': %w", err)
		}
		for _, item := range list.Records {
			if item.Domain != domains || item.StartDate != cert.NotBefore.Unix() || item.ExpireDate != cert.NotAfter.Unix() {
				continue
			}
			req := c.http.R().SetContext(ctx)
			detail, err := send[certificateDetail](req, http.MethodGet, fmt.Sprintf("/product/sslcenter/%d", item.ID))
			if err != nil {
				return 0, false, fmt.Errorf("failed to execute sdk request 'sslcenter.Get': %w", err)
			}
			if xcert.EqualCertificatesFromPEM(certPEM, detail.Cert) {
				return item.ID, true, nil
			}
		}
		if len(list.Records) < perPage {
			return 0, false, nil
		}
	}
}
