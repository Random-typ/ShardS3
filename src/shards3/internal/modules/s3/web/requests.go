package web

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"shards3/internal/platform/config"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Parses a request
// Returns the request if successful, nil otherwise.
func GetRequest[T any](r *http.Request) (*T, error) {
	var req T
	t := reflect.TypeOf(req)
	v := reflect.ValueOf(&req).Elem()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("http")
		defaultValue := field.Tag.Get("default")
		parts := strings.Split(tag, ",")
		if tag == "" || len(parts) != 2 {
			continue
		}
		httpName := parts[0]
		httpType := parts[1]

		var value string
		switch httpType {
		case "host":
			value, _ = bucketFromHost(r.Host, config.Cfg.FQDN)
		case "path":
			value = r.URL.Path
			// remove leading slash
			if len(value) > 0 && value[0] == '/' {
				value = value[1:]
			}
		case "query":
			value = r.URL.Query().Get(httpName)
		case "header":
			value = r.Header.Get(httpName)
		}

		if value == "" && defaultValue != "" {
			value = defaultValue
		}

		switch field.Type.Kind() {
		case reflect.Int:
			if value != "" {
				intValue, err := strconv.Atoi(value)
				if err != nil {
					return nil, fmt.Errorf("invalid integer value for field %s", field.Name)
				}
				v.FieldByName(field.Name).SetInt(int64(intValue))
			}
		case reflect.String:
			v.FieldByName(field.Name).SetString(value)
		case reflect.TypeOf(time.Time{}).Kind():
			if value != "" {
				timeValue, err := time.Parse(time.RFC1123, value)
				if err != nil {
					return nil, fmt.Errorf("invalid time value for field %s", field.Name)
				}
				v.FieldByName(field.Name).Set(reflect.ValueOf(timeValue))
			}
		}
	}
	return &req, nil
}

// Parses a request
// Handles S3 error response
// Returns request if successful, nil otherwise.
func HandleRequest[T any](w http.ResponseWriter, r *http.Request, bucketRequired bool, keyRequired bool, requiredHeaders []string, requiredQueries []string) *T {
	request, err := GetRequest[T](r)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), "")
		return nil
	}
	t := reflect.TypeOf(*request)
	v := reflect.ValueOf(*request)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		tag := field.Tag.Get("http")
		name := field.Tag.Get("name")
		rangeTag := field.Tag.Get("range")
		parts := strings.Split(tag, ",")
		ranges := strings.Split(rangeTag, ",")
		if tag == "" || len(parts) != 2 {
			continue
		}
		httpName := parts[0]
		httpType := parts[1]

		var rangeStart, rangeEnd int
		if rangeTag != "" && len(ranges) == 2 {
			rangeStart, _ = strconv.Atoi(ranges[0])
			rangeEnd, _ = strconv.Atoi(ranges[1])
		}

		if name == "" {
			name = httpName
		}

		switch httpType {
		case "host":
			if bucketRequired && value.String() == "" {
				writeS3Error(w, http.StatusBadRequest, "Invalid"+field.Name, name+" not found in host", "")
				return nil
			}
		case "path":
			if keyRequired && value.String() == "" {
				writeS3Error(w, http.StatusBadRequest, "Invalid"+field.Name, name+" not found in path", "")
				return nil
			}
		case "query":
			if slices.Contains(requiredQueries, httpName) && value.String() == "" {
				writeS3Error(w, http.StatusBadRequest, "Missing"+field.Name, name+" is required", "")
				return nil
			}
		case "header":
			if slices.Contains(requiredHeaders, httpName) && value.String() == "" {
				writeS3Error(w, http.StatusBadRequest, "Missing"+field.Name, name+" is required", "")
				return nil
			}
		}
		rangeProvided := false
		switch httpType {
		case "host":
			rawValue, _ := bucketFromHost(r.Host, config.Cfg.FQDN)
			rangeProvided = rawValue != ""
		case "path":
			rangeProvided = r.URL.Path != ""
		case "query":
			rangeProvided = r.URL.Query().Get(httpName) != ""
		case "header":
			rangeProvided = r.Header.Get(httpName) != ""
		}

		if field.Type.Kind() == reflect.Int && rangeTag != "" && rangeProvided && (value.Int() < int64(rangeStart) || value.Int() > int64(rangeEnd)) {
			writeS3Error(w, http.StatusBadRequest, "Invalid"+field.Name, name+" must be between "+strconv.Itoa(rangeStart)+" and "+strconv.Itoa(rangeEnd), "")
			return nil
		}
	}
	return request
}

func applyHeaders(w http.ResponseWriter, headers any) {
	if headers == nil {
		return
	}

	t := reflect.TypeOf(headers)
	v := reflect.ValueOf(headers)

	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		t = t.Elem()
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := field.Tag.Get("http")
		if name == "" {
			continue
		}
		value := v.Field(i)
		if field.Type.Kind() == reflect.TypeOf(time.Time{}).Kind() {
			w.Header().Set(name, value.Interface().(time.Time).UTC().Format(time.RFC1123))
			continue
		}

		switch value.Kind() {
		case reflect.String:
			w.Header().Set(name, value.String())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			w.Header().Set(name, strconv.FormatInt(value.Int(), 10))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			w.Header().Set(name, strconv.FormatUint(value.Uint(), 10))
		case reflect.Bool:
			w.Header().Set(name, strconv.FormatBool(value.Bool()))
		default:
			w.Header().Set(name, fmt.Sprint(value.Interface()))
		}
	}
}

func MarshalWithNamespace(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)

	start := xml.StartElement{
		Name: xml.Name{Local: reflect.TypeOf(v).Name()},
		Attr: []xml.Attr{
			{
				Name:  xml.Name{Local: "xmlns"},
				Value: "http://s3.amazonaws.com/doc/2006-03-01/",
			},
		},
	}

	if err := enc.EncodeElement(v, start); err != nil {
		return nil, err
	}

	if err := enc.Flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
func WriteResponse(w http.ResponseWriter, statusCode int, headers any, bodyXml any, data []byte) {
	if bodyXml != nil {
		w.Header().Set("Content-Type", "application/xml")
	}
	applyHeaders(w, headers)
	w.WriteHeader(statusCode)

	if bodyXml != nil {
		payload, err := MarshalWithNamespace(bodyXml)
		if err != nil {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", "failed to encode response", "")
			return
		}
		_, _ = w.Write([]byte(xml.Header))
		_, _ = w.Write(payload)
	}
	if data != nil {
		_, _ = w.Write(data)
	}
}

func WriteResponseStream(w http.ResponseWriter, statusCode int, headers any, bodyXml any, data io.Reader) error {
	if bodyXml != nil {
		w.Header().Set("Content-Type", "application/xml")
	}
	applyHeaders(w, headers)
	w.WriteHeader(statusCode)

	if bodyXml != nil {
		payload, err := MarshalWithNamespace(bodyXml)
		if err != nil {
			return fmt.Errorf("failed to encode response: %w", err)
		}
		if _, err := w.Write([]byte(xml.Header)); err != nil {
			return err
		}
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}

	if data != nil {
		if _, err := io.Copy(w, data); err != nil {
			return err
		}
	}

	return nil
}

func ParseContentRangeHeader(header string) (start int64, end int64, total int64, err error) {
	re := regexp.MustCompile(`^bytes(?:\s+|=)(\d+)-(\d+)(?:\/(\d+|\*))?$`)

	m := re.FindStringSubmatch(strings.TrimSpace(header))
	if m == nil {
		return 0, 0, 0, fmt.Errorf("invalid range header: %s", header)
	}

	start, err = strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid range header: %s", header)
	}

	inclusiveEnd, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid range header: %s", header)
	}
	end = inclusiveEnd + 1

	if len(m) >= 4 && m[3] != "" {
		if m[3] == "*" {
			total = -1
			return start, end, total, nil
		}

		total, err = strconv.ParseInt(m[3], 10, 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid range header: %s", header)
		}
		return start, end, total, nil
	}

	total = -1
	return start, end, total, nil
}
