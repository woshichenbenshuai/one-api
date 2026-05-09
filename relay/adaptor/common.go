package adaptor

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay/meta"
	"io"
	"net/http"
	"time"
)

func SetupCommonRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) {
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))
	if meta.IsStream && c.Request.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/event-stream")
	}
}

func DoRequestHelper(a Adaptor, c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.GetRequestURL(meta)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	if meta.UseResponsesCompat {
		logger.Infof(
			c.Request.Context(),
			"responses compat upstream request: %s %s (client_path=%s, model=%s)",
			c.Request.Method,
			fullRequestURL,
			c.Request.URL.Path,
			meta.ActualModelName,
		)
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	requestContext := c.Request.Context()
	cancel := func() {}
	if meta.Config.RequestTimeout > 0 {
		requestContext, cancel = context.WithTimeout(
			requestContext,
			time.Duration(meta.Config.RequestTimeout)*time.Second,
		)
		logger.Infof(
			requestContext,
			"using channel request timeout: %ds (channel_id=%d model=%s)",
			meta.Config.RequestTimeout,
			meta.ChannelId,
			meta.ActualModelName,
		)
	}
	defer cancel()
	req = req.WithContext(requestContext)
	err = a.SetupRequestHeader(c, req, meta)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	resp, err := DoRequest(c, req, meta)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	if meta.UseResponsesCompat {
		logger.Infof(
			c.Request.Context(),
			"responses compat upstream response: status=%d content_type=%s (model=%s)",
			resp.StatusCode,
			resp.Header.Get("Content-Type"),
			meta.ActualModelName,
		)
		logger.Infof(c.Request.Context(), "responses compat upstream headers: %v", resp.Header)
	}
	return resp, nil
}

func DoRequest(c *gin.Context, req *http.Request, meta *meta.Meta) (*http.Response, error) {
	relayHTTPClient := client.GetRelayHTTPClient(meta.Config.RequestTimeout)
	resp, err := relayHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("resp is nil")
	}
	_ = req.Body.Close()
	_ = c.Request.Body.Close()
	return resp, nil
}
