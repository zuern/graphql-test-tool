// Copyright (c) 2019, Peter Ohler, All rights reserved.

package gtt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Step is a step in a use case. Each describes how an HTTP request will be
// sent to a GraphQL server and what to do with the response.
type Step struct {

	// Label of the step as a way to refer to the step.
	Label string

	// Comment describes the step.
	Comment string

	// Path is the path part of the URL that will be used in the request to
	// the GraphQL server. The path is assumed to be relative to the base URL
	// unless it starts with a '/' character.
	Path string

	// Content is the content when using a POST request. If the Content is not
	// empty then a POST is used. If it is empty then a GET request is made to
	// the server. The string should be either GraphQL or the HTTP GET format.
	Content string

	// UseJSON if true and a POST request then the JSON format will be used
	// with a COntent-Type of "applicaiton/json" otherwise the Content will be
	// sent as GraphQL with a Content-Type of "application/graphql".
	UseJSON bool

	// Remember in a series of steps it is often helpful to remember values
	// from earlier steps. The Remember map describes what to remember and
	// what key to store that value in. In the map, the keys are the keys for
	// the memory cache while the Remember map values are the path to the
	// value to remember. The path is a simple dot delimited path. It is not a
	// full JSON path (maybe in the future).
	Remember map[string]string

	// Op is the operation to include in either the URL query or as a value
	// for the 'operationName' if using JSON in the Content.
	Op string

	// Vars are the variables to be passed along with a GraphQL request. They
	// are appended to the URL or added to the JSON 'variables' element if the
	// POST contents is JSON. The values in the Vars map can be either a value
	// or a string that begines with a '$' which indicates a rmembered value
	// should be used instead.
	Vars map[string]interface{}

	// SortBy are the sort keys for the result. Depending on the
	// implementation of the GraphQL server, the order of returned objects may
	// not be consistent. To make it easier to use in testing the SortBy keys
	// are the paths to arrays while the value for the keys are the attributes
	// to sort on.
	SortBy map[string]string

	// Expect is the expected contents. Like the Content it can be a nil, string,
	// array, or map. A nil value indicates no checking of the response is
	// needed. Strings are used mostly for fetched HTML while the normal JSON
	// responses are converted to a genereric map[string]interface{} and
	// compared to the Expect value.
	//
	// The rules for comparison are:
	//   1) Any element in the Expect value must be present in the response.
	//   2) Elements in the response not in the Expect value are ignored.
	//   3) If the Expect value is a string that starts and ends with a '/'
	//      character the string is assumed to be a regular expression is
	//      matched against the string in the reponse.
	//   4) Maps and arrays are followed recursively.
	Expect interface{}
}

func (s *Step) Set(data interface{}) (err error) {
	m, _ := data.(map[string]interface{})
	if m == nil {
		return fmt.Errorf("%T is not a valid type for a step", data)
	}
	var ok bool
	s.Label, _ = m["label"].(string)
	s.Path, _ = m["path"].(string)
	s.Op, _ = m["op"].(string)
	s.UseJSON, _ = m["json"].(bool)
	if s.Comment, err = asString(m["comment"]); err != nil {
		return
	}
	if v := m["content"]; v != nil {
		if s.Content, err = asString(v); err != nil {
			return
		}
	}
	if v := m["expect"]; v != nil {
		if s.Expect, err = asMapOrString(v); err != nil {
			return
		}
	}
	if v := m["remember"]; v != nil {
		if s.Remember, err = asMapStrStr(v); err != nil {
			return
		}
	}
	if v := m["sortBy"]; v != nil {
		if s.SortBy, err = asMapStrStr(v); err != nil {
			return
		}
	}
	if v := m["vars"]; v != nil {
		if s.Vars, ok = v.(map[string]interface{}); !ok {
			return fmt.Errorf("%T is not a valid type for a map[string]interface{}", v)
		}
	}
	return nil
}

func (s *Step) Native() interface{} {
	native := map[string]interface{}{
		"label": s.Label,
		"path":  s.Path,
	}
	if 0 < len(s.Comment) {
		native["comment"] = easyString(s.Comment)
	}
	if 0 < len(s.Op) {
		native["op"] = easyString(s.Op)
	}
	if 0 < len(s.Content) {
		native["content"] = easyString(s.Content)
	}
	if s.UseJSON {
		native["json"] = s.UseJSON
	}
	addAny(native, "expect", s.Expect)
	addNotNil(native, "remember", s.Remember)
	addNotNil(native, "vars", s.Vars)
	addNotNil(native, "sortBy", s.SortBy)

	return native
}

// Execute the step using the information in the provided use case such as
// remembered values and base URL.
func (s *Step) Execute(uc *UseCase) error {
	if len(uc.runner.Server) == 0 {
		return fmt.Errorf("server not specified")
	}
	var comment []string
	if 0 < len(s.Label) {
		comment = append(comment, s.Label)
	}
	if 0 < len(s.Comment) {
		comment = append(comment, s.Comment)
	}
	if 0 < len(comment) {
		uc.runner.Log(aComment, strings.Join(comment, ": "))
	}
	u := uc.runner.Server
	sep := '?'
	if 0 < len(s.Path) {
		if s.Path[0] != '/' { // relative path
			u += uc.runner.Base
		}
		u += s.Path
		sep = '&'
	} else {
		u += uc.runner.Base
	}
	vars := map[string]interface{}{}
	for k, v := range s.Vars {
		if s, _ := v.(string); 0 < len(s) && s[0] == '$' {
			v = uc.memory[s[1:]]
		}
		vars[k] = v
	}
	if s.UseJSON && len(s.Content) == 0 {
		return fmt.Errorf("if using JSON the content can not be empty in step %s", s.Label)
	}
	if !s.UseJSON {
		// Put the variables in the URL as a JSON string if not empty.
		if 0 < len(vars) {
			vb, err := json.Marshal(vars)
			if err != nil {
				return err
			}
			u = fmt.Sprintf("%s%cvariables=%s", u, sep, string(vb))
			sep = '&'
		}
		if 0 < len(s.Op) {
			u = fmt.Sprintf("%s%coperationName=%s", u, sep, s.Op)
		}
	}
	contentType := "application/graphql"
	var content io.Reader
	contentStr := s.Content
	if 0 < len(s.Content) { // POST
		// If the JSON then wrap and populate operationName and variables
		// otherwise add the variables and op to the URL query parameters.
		if s.UseJSON {
			contentType = "application/json"
			wrap := map[string]interface{}{
				"query": s.Content,
			}
			if 0 < len(vars) {
				wrap["variables"] = vars
			}
			if 0 < len(s.Op) {
				wrap["operationName"] = s.Op
			}
			if b, err := json.Marshal(wrap); err == nil {
				content = bytes.NewReader(b)
				contentStr = string(b)
			} else {
				return err
			}
		} else {
			content = strings.NewReader(s.Content)
		}
	}
	var res *http.Response
	var err error

	uc.runner.Log(aRequest, "URL: %s\nContent-Type: %s\n%s", u, contentType, contentStr)
	if content == nil {
		res, err = http.Get(u)
	} else {
		res, err = http.Post(u, contentType, content)
	}
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	if xstr, ok := s.Expect.(string); ok {
		return s.expectString(xstr, string(body), uc.runner)
	}
	return s.expectJSON(res.StatusCode, body, uc)
}

func (s *Step) expectJSON(status int, actual []byte, uc *UseCase) (err error) {
	var result interface{}
	if err = json.Unmarshal(actual, &result); err != nil {
		uc.runner.Log(aResponse, "[%d] %s", status, string(actual))
		return err
	}
	for path, key := range s.SortBy {
		s.sortResult(result, strings.Split(path, "."), key)
	}
	if uc.runner.ShowResponses {
		if 0 < uc.runner.Indent {
			j, _ := json.MarshalIndent(result, "", strings.Repeat(" ", uc.runner.Indent))
			uc.runner.Log(aResponse, "%s", string(j))
		} else {
			uc.runner.Log(aResponse, "%s", string(actual))
		}
	}
	for k, path := range s.Remember {
		s.remember(uc, result, k, strings.Split(path, "."))
	}
	if s.Expect != nil {
		if err = s.check(result); err != nil {
			return err
		}
	}
	return nil
}

func (s *Step) expectString(expect, actual string, r *Runner) (err error) {
	if expect == actual {
		r.Log(aResponse, "%s", actual)
		return nil
	}
	// Find were the mismatch is.
	var buf strings.Builder

	lines := strings.Split(actual, "\n")
	for i, xline := range strings.Split(expect, "\n") {
		if len(lines) <= i {
			err = fmt.Errorf("mismatch at line %d column 0", i+1)
			if !r.NoColor {
				buf.WriteString(normal)
				buf.WriteString(red)
				buf.WriteString("EOF")
			}
			break
		}
		aline := lines[i]
		if xline == aline {
			buf.WriteString(aline)
			buf.WriteByte('\n')
			continue
		}
		for j, xr := range []rune(xline) {
			if len([]rune(aline)) <= j {
				err = fmt.Errorf("mismatch at line %d column %d", i+1, j+1)
				if !r.NoColor {
					buf.WriteString(normal)
					buf.WriteString(red)
					buf.WriteByte('\n')
					buf.WriteString(strings.Join(lines[i+1:], "\n"))
				}
				break
			}
			ar := []rune(aline)[j]
			if xr == ar {
				buf.WriteRune(ar)
				continue
			}
			err = fmt.Errorf("mismatch at line %d column %d", i+1, j+1)
			if !r.NoColor {
				buf.WriteString(normal)
				buf.WriteString(red)
				buf.WriteString(aline[j:])
				buf.WriteByte('\n')
				buf.WriteString(strings.Join(lines[i+1:], "\n"))
			}
			break
		}
		break
	}
	if !r.NoColor {
		buf.WriteString(normal)
	}
	r.Log(aResponse, "%s", buf.String())

	return
}

func (s *Step) sortResult(result interface{}, path []string, key string) {
	if mr, ok := result.(map[string]interface{}); ok {
		if v := mr[path[0]]; v != nil {
			if 1 < len(path) {
				s.sortResult(v, path[1:], key)
			} else if a, _ := v.([]interface{}); a != nil {
				sort.Slice(a, func(i, j int) bool {
					var x string
					var y string
					if m, _ := a[i].(map[string]interface{}); m != nil {
						x, _ = m[key].(string)
					}
					if m, _ := a[j].(map[string]interface{}); m != nil {
						y, _ = m[key].(string)
					}
					return strings.Compare(x, y) < 0
				})
			}
		}
	}
}

func (s *Step) remember(uc *UseCase, result interface{}, rkey string, path []string) {
	switch tr := result.(type) {
	case map[string]interface{}:
		key := path[0]
		for k, v := range tr {
			if k == key {
				if len(path) == 1 {
					uc.memory[rkey] = v
					return
				}
				s.remember(uc, v, rkey, path[1:])
				break
			}
		}
	case []interface{}:
		if i, err := strconv.Atoi(path[0]); err == nil && 0 <= i && i < len(tr) {
			if len(path) == 1 && path[0] == rkey {
				uc.memory[rkey] = tr[i]
				return
			}
			s.remember(uc, tr[i], rkey, path[1:])
			break
		}
	}
}

func (s *Step) check(result interface{}) error {
	if loc, av, xv := match(result, s.Expect); loc != nil {
		return fmt.Errorf("%s result does not match expected at %s. %v != %v", s.Label, strings.Join(loc, "."), av, xv)
	}
	return nil
}
