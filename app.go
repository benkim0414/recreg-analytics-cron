package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/storage"
	recreg "github.com/benkim0414/go-recreg"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/appengine"
)

var (
	storageClient *storage.Client
	bucketName    = os.Getenv("GCLOUD_STORAGE_BUCKET")
)

func main() {
	ctx := context.Background()

	var err error
	creds, err := google.FindDefaultCredentials(ctx, storage.ScopeFullControl)
	if err != nil {
		log.Fatal(err)
	}

	storageClient, err = storage.NewClient(ctx, option.WithCredentials(creds))
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/_ah/health", healthCheckHandler)
	http.HandleFunc("/actions:upload", uploadHandler)
	appengine.Main()
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok")
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	var err error
	t := time.Now().AddDate(0, 0, -1)
	if v := r.URL.Query().Get("date"); v != "" {
		t, err = time.Parse(recreg.ISO8601Date, v)
		if err != nil {
			msg := fmt.Sprintf("could not parse date: %v", err)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
	}

	c := recreg.NewClient(nil)
	actions, err := c.ListActions(t)
	if err != nil {
		msg := fmt.Sprintf("could not get actions: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	b, err := json.Marshal(actions)
	if err != nil {
		msg := fmt.Sprintf("could not marshal actions: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	ctx := appengine.NewContext(r)

	fileName := t.Format(recreg.ISO8601Date) + ".json"
	sw := storageClient.Bucket(bucketName).Object(fileName).NewWriter(ctx)
	sw.ACL = []storage.ACLRule{
		{Entity: storage.AllUsers, Role: storage.RoleReader},
	}
	sw.ContentType = "application/json"
	if _, err := io.Copy(sw, bytes.NewReader(b)); err != nil {
		msg := fmt.Sprintf("could not write file: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	if err := sw.Close(); err != nil {
		msg := fmt.Sprintf("could not put file: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "https://storage.googleapis.com/%s/%s", bucketName, fileName)
}
