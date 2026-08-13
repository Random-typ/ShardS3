package web

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"shards3/services/shards3/internal/platform/config"
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
		if field.Type.Kind() == reflect.Int && rangeTag != "" && (value.Int() < int64(rangeStart) || value.Int() > int64(rangeEnd)) {
			writeS3Error(w, http.StatusBadRequest, "Invalid"+field.Name, name+" must be between "+strconv.Itoa(rangeStart)+" and "+strconv.Itoa(rangeEnd), "")
			return nil
		}
	}
	return request
}

func WriteResponse(w http.ResponseWriter, statusCode int, headers any, bodyXml any, data []byte) {
	w.WriteHeader(statusCode)

	if bodyXml != nil {
		payload, err := xml.MarshalIndent(bodyXml, "", "  ")
		if err != nil {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", "failed to encode response", "")
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(xml.Header))
		_, _ = w.Write(payload)
	}
	if data != nil {
		_, _ = w.Write(data)
	}

	if headers != nil {
		t := reflect.TypeOf(headers)
		v := reflect.ValueOf(headers).Elem()

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := field.Tag.Get("http")
			if name == "" {
				continue
			}
			if field.Type.Kind() == reflect.TypeOf(time.Time{}).Kind() {
				w.Header().Set(name, v.Field(i).Interface().(time.Time).UTC().Format(time.RFC1123))
				continue
			}
			w.Header().Set(name, v.Field(i).String())
		}
	}
}

func ParseContentRangeHeader(header string) (start int64, end int64, total int64, err error) {
	re := regexp.MustCompile(`bytes\s+(\d+)-(\d+)\/(\d+|\*)`)

	m := re.FindStringSubmatch(header)
	if m == nil {
		return 0, 0, 0, fmt.Errorf("invalid Content-Range header: %s", header)
	}

	start, err = strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid Content-Range header: %s", header)
	}
	end, err = strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid Content-Range header: %s", header)
	}
	if m[3] == "*" {
		total = -1
		return start, end, total, nil
	}
	total, err = strconv.ParseInt(m[3], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid Content-Range header: %s", header)
	}

	return start, end, total, nil
}
