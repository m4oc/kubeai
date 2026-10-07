package apiutils

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	k8sv1 "github.com/kubeai-project/kubeai/api/k8s/v1"
	"github.com/stretchr/testify/require"
)

type decisionModelClient struct {
	model     *k8sv1.Model
	err       error
	selectors []string
	calls     int
}

func (c *decisionModelClient) LookupModel(_ context.Context, model, adapter string, selectors []string) (*k8sv1.Model, error) {
	c.calls++
	c.selectors = selectors
	return c.model, c.err
}

func decisionModel(engine string) *k8sv1.Model {
	return &k8sv1.Model{Spec: k8sv1.ModelSpec{Engine: engine, Features: []k8sv1.ModelFeature{k8sv1.ModelFeatureSystemOne}}}
}

func TestSystemOneJSON(t *testing.T) {
	// The order of choices can affect inference; no decode/re-encode is allowed.
	body := ` {"model":"decision", "state":{"z":1,"a":2}, "questions":{"team":{"type":"choice","criteria":{"z":null,"a":"A"}}},"samples":"auto","chat_template_kwargs":{"enable_thinking":false}} `
	for _, engine := range []string{k8sv1.OLlamaEngine, k8sv1.VLLMEngine, k8sv1.LlamaCppEngine, k8sv1.SGLangEngine} {
		t.Run(engine, func(t *testing.T) {
			client := &decisionModelClient{model: decisionModel(engine)}
			headers := http.Header{"Content-Type": {"application/json; charset=utf-8"}, "X-Label-Selector": {"team=search"}}
			r, err := ParseRequest(context.Background(), client, strings.NewReader(body), SystemOnePath, headers)
			require.NoError(t, err)
			require.Equal(t, body, string(r.Body))
			require.Equal(t, int64(len(body)), r.ContentLength)
			require.Equal(t, "decision", r.RequestedModel)
			require.Equal(t, "decision", r.Model)
			require.Empty(t, r.Adapter)
			require.NotEmpty(t, r.ID)
			require.Equal(t, []string{"team=search"}, client.selectors)
		})
	}
}

func TestSystemOneRejections(t *testing.T) {
	for _, tc := range []struct{ name, body, contentType string }{
		{"malformed", `{`, "application/json"},
		{"missing model", `{}`, "application/json"},
		{"blank model", `{"model":" "}`, "application/json"},
		{"null model", `{"model":null}`, "application/json"},
		{"number model", `{"model":12}`, "application/json"},
		{"duplicate model", `{"model":"one","model":"two"}`, "application/json"},
		{"trailing JSON", `{"model":"one"}{}`, "application/json"},
		{"adapter", `{"model":"one_adapter"}`, "application/json"},
		{"native adapter", `{"model":"one:adapter"}`, "application/json"},
		{"unsupported content type", `{"model":"one"}`, "text/plain"},
		{"missing boundary", "", "multipart/form-data"},
		{"malformed multipart", "broken", "multipart/form-data; boundary=test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &decisionModelClient{model: decisionModel(k8sv1.VLLMEngine)}
			_, err := ParseRequest(context.Background(), client, strings.NewReader(tc.body), SystemOnePath, http.Header{"Content-Type": {tc.contentType}})
			require.ErrorIs(t, err, ErrBadRequest)
			require.Zero(t, client.calls)
		})
	}
	for _, engine := range []string{k8sv1.InfinityEngine, k8sv1.FasterWhisperEngine, "", "future-engine"} {
		client := &decisionModelClient{model: decisionModel(engine)}
		_, err := ParseRequest(context.Background(), client, strings.NewReader(`{"model":"one"}`), SystemOnePath, nil)
		require.ErrorIs(t, err, ErrBadRequest, engine)
	}
	for _, engine := range []string{k8sv1.OLlamaEngine, k8sv1.VLLMEngine, k8sv1.LlamaCppEngine, k8sv1.SGLangEngine} {
		m := decisionModel(engine)
		m.Spec.Features = []k8sv1.ModelFeature{k8sv1.ModelFeatureTextGeneration}
		_, err := ParseRequest(context.Background(), &decisionModelClient{model: m}, strings.NewReader(`{"model":"one"}`), SystemOnePath, nil)
		require.ErrorIs(t, err, ErrBadRequest)
	}
	_, err := ParseRequest(context.Background(), &decisionModelClient{}, strings.NewReader(`{"model":"missing"}`), SystemOnePath, nil)
	require.ErrorIs(t, err, ErrModelNotFound)
	lookupErr := errors.New("lookup failed")
	_, err = ParseRequest(context.Background(), &decisionModelClient{err: lookupErr}, strings.NewReader(`{"model":"one"}`), SystemOnePath, nil)
	require.ErrorIs(t, err, lookupErr)
}

func TestSystemOneMultipart(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		file, err := writer.CreateFormFile("image", "photo.png")
		require.NoError(t, err)
		_, err = file.Write([]byte{0, 255, 1, 2, 13, 10})
		require.NoError(t, err)
		for i := 0; i < count; i++ {
			require.NoError(t, writer.WriteField("request", `{"model":"decision","state":"","questions":{},"images":[]}`))
		}
		require.NoError(t, writer.Close())
		r, err := ParseRequest(context.Background(), &decisionModelClient{model: decisionModel(k8sv1.VLLMEngine)}, bytes.NewReader(body.Bytes()), SystemOnePath, http.Header{"Content-Type": {writer.FormDataContentType()}})
		if count != 1 {
			require.ErrorIs(t, err, ErrBadRequest)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, body.Bytes(), r.Body)
		require.Equal(t, int64(body.Len()), r.ContentLength)
		require.Equal(t, "decision", r.Model)
	}
}

type repeatedByteReader struct{}

func (repeatedByteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

func TestSystemOneBodyLimit(t *testing.T) {
	client := &decisionModelClient{}
	_, err := ParseRequest(context.Background(), client, io.LimitReader(repeatedByteReader{}, MaxSystemOneBodyBytes+1), SystemOnePath, nil)
	require.ErrorIs(t, err, ErrRequestTooLarge)
	require.Zero(t, client.calls)
}

func TestSystemOneMultipartRequestFileRejected(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("request", "request.json")
	require.NoError(t, err)
	_, err = io.WriteString(part, `{"model":"decision"}`)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	client := &decisionModelClient{model: decisionModel(k8sv1.OLlamaEngine)}
	_, err = ParseRequest(context.Background(), client, &body, SystemOnePath, http.Header{"Content-Type": {writer.FormDataContentType()}})
	require.ErrorIs(t, err, ErrBadRequest)
	require.Zero(t, client.calls)
}

func TestSystemOneMultipartAllEngines(t *testing.T) {
	for _, engine := range []string{k8sv1.OLlamaEngine, k8sv1.VLLMEngine, k8sv1.LlamaCppEngine, k8sv1.SGLangEngine} {
		t.Run(engine, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("request", `{"model":"decision","engine_extension":{"precision":1.00}}`))
			file, err := writer.CreateFormFile("image", "input.bin")
			require.NoError(t, err)
			_, err = file.Write([]byte{0, 255, 13, 10})
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			raw := bytes.Clone(body.Bytes())
			client := &decisionModelClient{model: decisionModel(engine)}
			r, err := ParseRequest(context.Background(), client, &body, SystemOnePath, http.Header{"Content-Type": {writer.FormDataContentType()}})
			require.NoError(t, err)
			require.Equal(t, raw, r.Body)
			require.Equal(t, int64(len(raw)), r.ContentLength)
			require.Equal(t, "decision", r.Model)
			require.Equal(t, 1, client.calls)
		})
	}
}

func TestSystemOneBodyAtLimit(t *testing.T) {
	const envelope = `{"model":"decision"}`
	body := envelope + strings.Repeat(" ", int(MaxSystemOneBodyBytes)-len(envelope))
	client := &decisionModelClient{model: decisionModel(k8sv1.LlamaCppEngine)}
	r, err := ParseRequest(context.Background(), client, strings.NewReader(body), SystemOnePath, nil)
	require.NoError(t, err)
	require.Equal(t, MaxSystemOneBodyBytes, r.ContentLength)
	require.Equal(t, body, string(r.Body))
	require.Equal(t, 1, client.calls)
}

func TestSystemOneDoesNotValidateEnginePayload(t *testing.T) {
	for _, engine := range []string{k8sv1.OLlamaEngine, k8sv1.VLLMEngine, k8sv1.LlamaCppEngine, k8sv1.SGLangEngine} {
		t.Run(engine, func(t *testing.T) {
			// KubeAI validates model routing only. Even a payload rejected by an
			// engine must reach that engine so it can return its own diagnostics.
			body := `{"model":"decision","state":[false,1.00],"questions":"backend-defined","extension":{"z":null,"a":true}}`
			client := &decisionModelClient{model: decisionModel(engine)}
			r, err := ParseRequest(context.Background(), client, strings.NewReader(body), SystemOnePath, nil)
			require.NoError(t, err)
			require.Equal(t, body, string(r.Body))
			require.Equal(t, 1, client.calls)
		})
	}
}
