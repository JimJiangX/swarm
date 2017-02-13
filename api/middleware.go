package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/Sirupsen/logrus"
)

// DebugRequestMiddleware dumps the request to logger
func DebugRequestMiddleware(handler func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || logrus.GetLevel() != logrus.DebugLevel {
			handler(w, r)
			return
		}
		if err := checkForJSON(r); err != nil {
			handler(w, r)
			return
		}
		maxBodySize := 4096 // 4KB
		if r.ContentLength > int64(maxBodySize) {
			handler(w, r)
			return
		}

		body := r.Body
		bufReader := bufio.NewReaderSize(body, maxBodySize)
		r.Body = newReadCloserWrapper(bufReader, func() error { return body.Close() })

		b, err := bufReader.Peek(maxBodySize)
		if err != io.EOF {
			// either there was an error reading, or the buffer is full (in which case the request is too large)
			handler(w, r)
			return
		}

		var postForm map[string]interface{}
		if err := json.Unmarshal(b, &postForm); err == nil {
			maskSecretKeys(postForm)
			formStr, errMarshal := json.Marshal(postForm)
			if errMarshal == nil {
				logrus.Debugf("form data: %s", string(formStr))
			} else {
				logrus.Debugf("form data: %q", postForm)
			}
		}

		handler(w, r)
		return
	}
}

func maskSecretKeys(inp interface{}) {
	if arr, ok := inp.([]interface{}); ok {
		for _, f := range arr {
			maskSecretKeys(f)
		}
		return
	}
	if form, ok := inp.(map[string]interface{}); ok {
	loop0:
		for k, v := range form {
			for _, m := range []string{"password", "secret", "jointoken", "unlockkey"} {
				if strings.EqualFold(m, k) {
					form[k] = "*****"
					continue loop0
				}
			}
			maskSecretKeys(v)
		}
	}
}

// checkForJSON makes sure that the request's Content-Type is application/json.
func checkForJSON(r *http.Request) error {
	ct := r.Header.Get("Content-Type")

	// No Content-Type header is ok as long as there's no Body
	if ct == "" {
		if r.Body == nil || r.ContentLength == 0 {
			return nil
		}
	}

	// Otherwise it better be json
	if matchesContentType(ct, "application/json") {
		return nil
	}
	return fmt.Errorf("Content-Type specified (%s) must be 'application/json'", ct)
}

// matchesContentType validates the content type against the expected one
func matchesContentType(contentType, expectedType string) bool {
	mimetype, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		logrus.Errorf("Error parsing media type: %s error: %v", contentType, err)
	}
	return err == nil && mimetype == expectedType
}

type readCloserWrapper struct {
	io.Reader
	closer func() error
}

func (r *readCloserWrapper) Close() error {
	return r.closer()
}

// newReadCloserWrapper returns a new io.ReadCloser.
func newReadCloserWrapper(r io.Reader, closer func() error) io.ReadCloser {
	return &readCloserWrapper{
		Reader: r,
		closer: closer,
	}
}
