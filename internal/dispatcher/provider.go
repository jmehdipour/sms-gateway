package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jmehdipour/sms-gateway/internal/model"
)

type Provider interface {
	Name() string
	Ready() bool
	Acquire() bool
	SendNormal(ctx context.Context, sms model.SMS) error
	SendExpress(ctx context.Context, sms model.SMS) error
}

type HTTPProvider struct {
	name        string
	baseURL     string
	normalPath  string
	expressPath string
	client      *http.Client
	br          *MicroBreaker
}

func NewHTTPProvider(
	name, baseURL, normalPath, expressPath string,
	timeoutMs, failThreshold, openForMs int,
) *HTTPProvider {
	if timeoutMs <= 0 {
		timeoutMs = 3000
	}

	if failThreshold <= 0 {
		failThreshold = 3
	}

	if openForMs <= 0 {
		openForMs = 15000
	}

	return &HTTPProvider{
		name:        name,
		baseURL:     baseURL,
		normalPath:  normalPath,
		expressPath: expressPath,
		client:      &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond},
		br:          NewMicroBreaker(failThreshold, time.Duration(openForMs)*time.Millisecond),
	}
}

func (p *HTTPProvider) Name() string  { return p.name }
func (p *HTTPProvider) Ready() bool   { return p.br.Ready() }
func (p *HTTPProvider) Acquire() bool { return p.br.TryAcquire() }

func (p *HTTPProvider) SendNormal(ctx context.Context, sms model.SMS) error {
	if err := p.post(ctx, p.normalPath, sms); err != nil {
		p.br.OnFailure()
		return err
	}

	p.br.OnSuccess()

	return nil
}

func (p *HTTPProvider) SendExpress(ctx context.Context, sms model.SMS) error {
	if err := p.post(ctx, p.expressPath, sms); err != nil {
		p.br.OnFailure()
		return err
	}

	p.br.OnSuccess()

	return nil
}

func (p *HTTPProvider) post(ctx context.Context, path string, sms model.SMS) error {
	b, _ := json.Marshal(sms)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode/100 != 2 {
		return fmt.Errorf("provider=%s path=%s status=%d", p.name, path, res.StatusCode)
	}

	return nil
}
