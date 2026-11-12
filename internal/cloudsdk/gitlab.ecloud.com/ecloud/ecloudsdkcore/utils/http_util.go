package utils

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
)

var (
	jsonCheck = regexp.MustCompile("(?i:(?:application|text)/json)")
	xmlCheck  = regexp.MustCompile("(?i:(?:application|text)/xml)")
)

func DetectContentType(body interface{}) string {
	contentType := "text/plain; charset=utf-8"
	kind := reflect.TypeOf(body).Kind()

	switch kind {
	case reflect.Struct, reflect.Map, reflect.Ptr:
		contentType = "application/json; charset=utf-8"
	case reflect.String:
		contentType = "text/plain; charset=utf-8"
	default:
		if b, ok := body.([]byte); ok {
			contentType = http.DetectContentType(b)
		} else if kind == reflect.Slice {
			contentType = "application/json; charset=utf-8"
		}
	}

	return contentType
}

func SetBody(body interface{}, contentType string) (bodyBuf *bytes.Buffer, err error) {
	if bodyBuf == nil {
		bodyBuf = &bytes.Buffer{}
	}
	switch v := body.(type) {
	case io.Reader:
		_, err = bodyBuf.ReadFrom(v)
	case []byte:
		_, err = bodyBuf.Write(v)
	case string:
		_, err = bodyBuf.WriteString(v)
	case *string:
		_, err = bodyBuf.WriteString(*v)
	default:
		if jsonCheck.MatchString(contentType) {
			bodyType := reflect.TypeOf(body)
			if bodyType.Kind() == reflect.Ptr {
				bodyType = bodyType.Elem()
			}
			if bodyType.NumField() == 1 && bodyType.Field(0).Type.Kind() == reflect.Slice {
				if IsUnSet(bodyType.Field(0).Tag.Get("json")) {
					v := reflect.ValueOf(body)
					if v.Kind() == reflect.Ptr {
						v = v.Elem()
					}
					body = v.Field(0).Interface()
				}
			}
			err = json.NewEncoder(bodyBuf).Encode(body)
		} else if xmlCheck.MatchString(contentType) {
			err = xml.NewEncoder(bodyBuf).Encode(body)
		}
	}
	if err != nil {
		return nil, err
	}
	if bodyBuf.Len() == 0 {
		err = fmt.Errorf("Invalid body type %s", contentType)
		return nil, err
	}
	return bodyBuf, nil
}
