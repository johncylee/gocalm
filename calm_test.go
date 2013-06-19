// Copyright 2013 John Lee <john@0xlab.org>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gocalm

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/johncylee/goroute"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type KeyValue struct {
	Key   string
	Value string
}

var dataStore map[string]string = map[string]string{
	"Peter": "Lemon",
	"Paul":  "Tree",
	"Mary":  "Very Pretty",
}

type Model struct {
}

func (m *Model) Get(key string) (interface{}, error) {
	s := dataStore[key]
	if s == "" {
		return nil, nil // Not found
	}
	return &KeyValue{
		Key:   key,
		Value: s,
	}, nil
}

func (m *Model) GetAll() (interface{}, error) {
	c := make(chan interface{})
	go func() {
		for n, v := range dataStore {
			c <- &KeyValue{Key: n, Value: v}
		}
		close(c)
	}()
	return c, nil
}

func (m *Model) Put(key string, v interface{}) (err error) {
	f, ok := v.(*KeyValue)
	if !ok {
		return TypeMismatch
	}
	if dataStore[key] == "" {
		return NotFound
	}
	dataStore[key] = f.Value
	return nil
}

func (m *Model) PutAll(v interface{}) (err error) {
	a, ok := v.([]KeyValue)
	if !ok {
		return TypeMismatch
	}
	dataStore = make(map[string]string)
	for _, f := range a {
		dataStore[f.Key] = f.Value
	}
	return nil
}

func (m *Model) Post(v interface{}) (string, error) {
	f, ok := v.(*KeyValue)
	if !ok {
		return "", TypeMismatch
	}
	if dataStore[f.Key] != "" {
		return "", errors.New("Already exists")
	}
	dataStore[f.Key] = f.Value
	return f.Key, nil
}

func (m *Model) Delete(key string) (err error) {
	if dataStore[key] == "" {
		return NotFound
	}
	delete(dataStore, key)
	return nil
}

func (m *Model) DeleteAll() (err error) {
	dataStore = map[string]string{}
	return nil
}

func Expect(t *testing.T, r *http.Response, v interface{}) {
	switch expect := v.(type) {
	case []byte:
		b := r.Body
		defer b.Close()
		body, err := ioutil.ReadAll(b)
		if err != nil {
			t.Error(err)
			return
		}
		if !reflect.DeepEqual(body, expect) {
			t.Errorf("Expect: `%s', got: `%s'\n", expect, body)
			return
		}
		log.Printf("Got expected response: `%s'\n", expect)
		return
	case int:
		if r.StatusCode != expect {
			t.Errorf("Expect %d, got %d\n", expect, r.StatusCode)
			return
		}
		log.Printf("Got expected status: %d\n", expect)
		return
	}
	t.Fatal("Unexpected type")
}

func VerifyGet(t *testing.T, s *httptest.Server, id string) {
	client := http.Client{}
	req, err := http.NewRequest(`GET`, s.URL+`/`+id, nil)
	if err != nil {
		t.Error(err)
		return
	}
	req.Header.Set(`Accept`, `application/json`)
	res, err := client.Do(req)
	if err != nil {
		t.Error(err)
		return
	}
	v := dataStore[id]
	if v == "" {
		Expect(t, res, http.StatusNotFound)
		return
	}
	j, _ := json.Marshal(KeyValue{id, dataStore[id]})
	Expect(t, res, j)
	req.Header.Set(`Accept`, `text/html`)
	res, err = client.Do(req)
	if err != nil {
		t.Error(err)
		return
	}
	Expect(t, res, http.StatusNotAcceptable)
}

func TestRestful(t *testing.T) {
	h := RESTHandler{
		Name:       "test",
		Model:      &Model{},
		DataType:   reflect.TypeOf(KeyValue{}),
		Expiration: 5, // expires in 5 seconds
	}
	s := httptest.NewServer(goroute.Handle(
		"/", `(?P<key>[[:alnum:]]*)`, &h))
	defer s.Close()
	// GET each
	for _, id := range []string{"Peter", "Paul", "Mary"} {
		VerifyGet(t, s, id)
	}
	// GET /
	res, err := http.Get(s.URL)
	if err != nil {
		t.Error(err)
	} else {
		tmpKeyValues := make([]KeyValue, len(dataStore))
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		err = json.Unmarshal(body, &tmpKeyValues)
		if err != nil {
			t.Fatal(err)
		}
		tmpDataStore := make(map[string]string)
		for _, v := range tmpKeyValues {
			tmpDataStore[v.Key] = v.Value
		}
		if reflect.DeepEqual(tmpDataStore, dataStore) {
			log.Println("All data retrieved correctly")
		} else {
			t.Errorf("%s != %s", tmpDataStore, dataStore)
		}
	}
	// PUT /Peter
	client := http.Client{}
	req, err := http.NewRequest(`PUT`, s.URL+"/Peter",
		strings.NewReader(`{"Value":"Orange!"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(`Content-Type`, `application/json`)
	res, err = client.Do(req)
	if err != nil {
		t.Error(err)
	} else {
		Expect(t, res, []byte("OK"))
	}
	// GET /Peter to verify
	VerifyGet(t, s, "Peter")
	// POST
	j, _ := json.Marshal(KeyValue{"JohnSmith", "Stranger"})
	req, err = http.NewRequest(`POST`, s.URL, bytes.NewReader(j))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(`Content-Type`, `application/json`)
	res, err = client.Do(req)
	if err != nil {
		t.Error(err)
	} else {
		Expect(t, res, []byte("JohnSmith"))
	}
	// GET /JohnSmith to verify
	VerifyGet(t, s, "JohnSmith")
	// DELETE /Paul
	req, err = http.NewRequest(`DELETE`, s.URL+"/Paul", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err = client.Do(req)
	if err != nil {
		t.Error(err)
	} else {
		Expect(t, res, []byte("OK"))
	}
	// GET /Paul to verify
	VerifyGet(t, s, "Paul")
	// DELETE /{Peter,Mary,JohnSmith}
	for _, id := range []string{"Peter", "Mary", "JohnSmith"} {
		req, err = http.NewRequest(`DELETE`, s.URL+"/"+id, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err = client.Do(req)
		if err != nil {
			t.Error(err)
		} else {
			Expect(t, res, []byte("OK"))
		}
	}
	// GET / to verify
	res, err = http.Get(s.URL)
	if err != nil {
		t.Error(err)
	} else {
		Expect(t, res, []byte("[]"))
	}
}