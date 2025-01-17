/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package credentialstatus

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"

	"github.com/hyperledger/aries-framework-go/pkg/doc/verifiable"
	vdrapi "github.com/hyperledger/aries-framework-go/pkg/framework/aries/api/vdr"
	"github.com/piprate/json-gold/ld"
	"github.com/trustbloc/edge-core/pkg/log"
)

var logger = log.New("vcs-verifier-restapi-v1")

const (
	cslRequestTokenName = "csl"
)

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Config struct {
	VDR            vdrapi.Registry
	TLSConfig      *tls.Config
	RequestTokens  map[string]string
	DocumentLoader ld.DocumentLoader
}

type Service struct {
	vdr            vdrapi.Registry
	httpClient     httpClient
	requestTokens  map[string]string
	documentLoader ld.DocumentLoader
}

func New(config *Config) *Service {
	return &Service{
		vdr:            config.VDR,
		httpClient:     &http.Client{Transport: &http.Transport{TLSClientConfig: config.TLSConfig}},
		requestTokens:  config.RequestTokens,
		documentLoader: config.DocumentLoader,
	}
}

func (s *Service) GetRevocationListVC(statusURL string) (*verifiable.Credential, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.sendHTTPRequest(req, http.StatusOK, s.requestTokens[cslRequestTokenName])
	if err != nil {
		return nil, err
	}

	revocationListVC, err := s.parseAndVerifyVC(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse and verify status vc: %w", err)
	}

	return revocationListVC, nil
}

func (s *Service) parseAndVerifyVC(vcBytes []byte) (*verifiable.Credential, error) {
	vc, err := verifiable.ParseCredential(
		vcBytes,
		verifiable.WithPublicKeyFetcher(
			verifiable.NewVDRKeyResolver(s.vdr).PublicKeyFetcher(),
		),
		verifiable.WithJSONLDDocumentLoader(s.documentLoader),
	)
	if err != nil {
		return nil, err
	}

	return vc, nil
}

func (s *Service) sendHTTPRequest(req *http.Request, status int, token string) ([]byte, error) {
	if token != "" {
		req.Header.Add("Authorization", "Bearer "+token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		err = resp.Body.Close()
		if err != nil {
			logger.Warnf("failed to close response body")
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Warnf("failed to read response body for status %d: %s", resp.StatusCode, err)
	}

	if resp.StatusCode != status {
		return nil, fmt.Errorf("failed to read response body for status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
