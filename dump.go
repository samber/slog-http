package sloghttp

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
)

var _ WrapResponseWriter = (*bodyWriter)(nil)

type WrapResponseWriter interface {
	http.ResponseWriter
	http.Flusher
	http.Hijacker

	Status() int
	BytesWritten() int
	Body() []byte
}

var _ http.ResponseWriter = (*bodyWriter)(nil)
var _ http.Flusher = (*bodyWriter)(nil)
var _ http.Hijacker = (*bodyWriter)(nil)

type bodyWriter struct {
	http.ResponseWriter
	body    *bytes.Buffer
	maxSize int
	bytes   int
	status  int
}

// implements http.ResponseWriter
func (w *bodyWriter) Write(b []byte) (int, error) {
	if w.body != nil {
		if w.body.Len()+len(b) > w.maxSize {
			w.body.Truncate(min(w.maxSize, len(b), w.body.Len()))
			w.body.Write(b[:w.maxSize-w.body.Len()])
		} else {
			w.body.Write(b)
		}
	}
	w.bytes += len(b) //nolint:staticcheck
	return w.ResponseWriter.Write(b)
}

// implements http.ResponseWriter
func (r *bodyWriter) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// implements http.Flusher
func (w *bodyWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// implements http.Hijacker
func (w *bodyWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hi, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hi.Hijack()
	}

	return nil, nil, errors.New("Hijack not supported")
}

// Unwrap implements the ability to use underlying http.ResponseController
func (w *bodyWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *bodyWriter) Status() int {
	return w.status
}

func (w *bodyWriter) BytesWritten() int {
	return w.bytes
}

func (w *bodyWriter) Body() []byte {
	return w.body.Bytes()
}

func newBodyWriter(writer http.ResponseWriter, maxSize int, recordBody bool) *bodyWriter {
	var body *bytes.Buffer
	if recordBody {
		body = bytes.NewBufferString("")
	}

	return &bodyWriter{
		ResponseWriter: writer,
		body:           body,
		maxSize:        maxSize,
		bytes:          0,
		status:         http.StatusOK,
	}
}

type bodyReader struct {
	io.ReadCloser
	body    *bytes.Buffer
	maxSize int
	bytes   int
}

// implements io.Reader
func (r *bodyReader) Read(b []byte) (int, error) {
	n, err := r.ReadCloser.Read(b)
	if r.body != nil && r.body.Len() < r.maxSize {
		if r.body.Len()+n > r.maxSize {
			r.body.Write(b[:r.maxSize-r.body.Len()])
		} else {
			r.body.Write(b[:n])
		}
	}
	r.bytes += n
	return n, err
}

func newBodyReader(reader io.ReadCloser, maxSize int, recordBody bool) *bodyReader {
	var body *bytes.Buffer
	if recordBody {
		body = new(bytes.Buffer)
	}

	return &bodyReader{
		ReadCloser: reader,
		body:       body,
		maxSize:    maxSize,
		bytes:      0,
	}
}
