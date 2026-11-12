package utils

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

type cacheControl map[string]string

func parseCacheControl(headers http.Header) cacheControl {
	cc := cacheControl{}
	ccHeader := headers.Get("Cache-Control")
	for _, part := range strings.Split(ccHeader, ",") {
		part = strings.Trim(part, " ")
		if part == "" {
			continue
		}
		if strings.ContainsRune(part, '=') {
			keyval := strings.Split(part, "=")
			cc[strings.Trim(keyval[0], " ")] = strings.Trim(keyval[1], ",")
		} else {
			cc[part] = ""
		}
	}
	return cc
}

func CacheExpires(r *http.Response) time.Time {
	var expires time.Time
	now, err := time.Parse(time.RFC1123, r.Header.Get("date"))
	if err != nil {
		return time.Now()
	}
	respCacheControl := parseCacheControl(r.Header)

	if maxAge, ok := respCacheControl["max-age"]; ok {
		lifetime, err := time.ParseDuration(maxAge + "s")
		if err != nil {
			expires = now
		}
		expires = now.Add(lifetime)
	} else {
		expiresHeader := r.Header.Get("Expires")
		if expiresHeader != "" {
			expires, err = time.Parse(time.RFC1123, expiresHeader)
			if err != nil {
				expires = now
			}
		}
	}
	return expires
}

func addFile(w *multipart.Writer, fieldName, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	part, err := w.CreateFormFile(fieldName, filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)

	return err
}

func selectHeaderContentType(contentTypes []string) string {
	if len(contentTypes) == 0 {
		return ""
	}
	if contains(contentTypes, "application/json") {
		return "application/json"
	}
	return contentTypes[0]
}

func selectHeaderAccept(accepts []string) string {
	if len(accepts) == 0 {
		return ""
	}

	if contains(accepts, "application/json") {
		return "application/json"
	}

	return strings.Join(accepts, ",")
}

func contains(haystack []string, needle string) bool {
	for _, a := range haystack {
		if strings.ToLower(a) == strings.ToLower(needle) {
			return true
		}
	}
	return false
}

func typeCheckParameter(obj interface{}, expected string, name string) error {
	if obj == nil {
		return nil
	}
	if reflect.TypeOf(obj).String() != expected {
		return fmt.Errorf("expected %s to be of type %s but received %s", name, expected, reflect.TypeOf(obj).String())
	}
	return nil
}

func IsSet(val interface{}) bool {
	if val == nil {
		return false
	}
	v := reflect.ValueOf(val)
	if v.Kind() == reflect.Ptr || v.Kind() == reflect.Slice || v.Kind() == reflect.Map {
		return !v.IsNil()
	}
	valType := reflect.TypeOf(val)
	valZero := reflect.Zero(valType)
	return valZero != v
}

func IsUnSet(val interface{}) bool {
	return !IsSet(val)
}

func DeepCopy(dst, src interface{}) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(src); err != nil {
		return err
	}
	return gob.NewDecoder(bytes.NewBuffer(buf.Bytes())).Decode(dst)
}
