// Copyright 2016 Henry Donnay. All rights reserved.
// Use of this source code is governed by an ISC-style
// license that can be found in the LICENSE file.

// This is a mutt "query_command" for querying Google Contacts.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	people "google.golang.org/api/people/v1"
)

var (
	tokenFile = os.ExpandEnv("$HOME/.contacts-token")
)

func main() {
	rd, wr := io.Pipe()
	printed := false
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	flag.Parse()

	sj, err := ioutil.ReadFile(os.ExpandEnv("$HOME/.contacts-secrets.json"))
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := google.ConfigFromJSON(sj, "profile", people.ContactsScope)
	if err != nil {
		log.Fatal(err)
	}
	tok, err := getToken(cfg)
	if err != nil {
		log.Fatal(err)
	}
	c := cfg.Client(oauth2.NoContext, tok)

	p, err := people.New(c)
	if err != nil {
		log.Fatal(err)
	}

	q := ""
	if flag.NArg() != 0 {
		q = flag.Arg(0)
	}

	s := bufio.NewScanner(rd)

	go func() {
		defer wr.Close()
		fmt.Printf("fetching...\t")
		lres, err := p.People.Connections.List("people/me").
			PageSize(500).
			RequestMaskIncludeField("person.names,person.email_addresses").
			Do()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		} else {
			fmt.Println("OK")
		}
		for _, p := range lres.Connections {
			if len(p.EmailAddresses) == 0 {
				continue
			}
			printPerson(wr, p)
		}
	}()

	for s.Scan() {
		if strings.Contains(s.Text(), q) {
			fmt.Println(s.Text())
			printed = true
		}
	}
	if err := s.Err(); err != nil {
		log.Fatal(err)
	}
	if !printed {
		os.Exit(1)
	}
}

func printPerson(w io.Writer, p *people.Person) {
	if len(p.Names) == 0 {
		for _, a := range p.EmailAddresses {
			fmt.Fprintf(w, "%s\t%s\t\n", a.Value, strings.Split(a.Value, "@")[0])
		}
		return
	}
	name := p.Names[0].DisplayName
	for _, a := range p.EmailAddresses {
		fmt.Fprintf(w, "%s\t%s\t\n", a.Value, name)
	}
}

func getToken(cfg *oauth2.Config) (*oauth2.Token, error) {
	var err error
	var src oauth2.TokenSource
	if src, err = newTokenFile(tokenFile); err != nil {

		code := make(chan string)
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := r.FormValue("code")
			if c != "" {
				code <- c
			} else {
				srv.Close()
			}
			io.Copy(w, strings.NewReader(`<html><pre>close me</pre><html>`))
		}))

		cfg.RedirectURL = srv.URL
		u := cfg.AuthCodeURL("csrf", oauth2.AccessTypeOffline)
		if err := open(u); err != nil {
			log.Fatal(err)
		}

		tok, err := cfg.Exchange(oauth2.NoContext, <-code)
		if err != nil {
			log.Fatal(err)
		}

		f, err := os.Create(tokenFile)
		if err != nil {
			return nil, err
		}
		if err := json.NewEncoder(f).Encode(tok); err != nil {
			return nil, err
		}

		return tok, nil
	}

	tok, err := src.Token()
	if err != nil {
		return nil, err
	}
	return tok, nil
}

type tokenfile struct {
	sync.Mutex
	os.File
}

func newTokenFile(name string) (*tokenfile, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return &tokenfile{File: *f}, nil
}

func (t *tokenfile) Token() (*oauth2.Token, error) {
	t.Lock()
	defer t.Unlock()

	if _, err := t.File.Seek(0, 0); err != nil {
		return nil, err
	}
	tok := oauth2.Token{}
	if err := json.NewDecoder(t).Decode(&tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func open(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return fmt.Errorf("dunno how to open URLs")
	}
}
