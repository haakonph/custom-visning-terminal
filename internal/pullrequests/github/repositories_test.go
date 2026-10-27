package github

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"
)

func TestWritableRepositoriesPaginationAndExplicitPermissions(t *testing.T) {
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("page") == "1" {
			w.Header().Set("Link", fmt.Sprintf(`<http://%s/user/repos?page=2>; rel="next"`, r.Host))
			fmt.Fprint(w, `[{"full_name":"acme/write","permissions":{"push":true}},{"full_name":"acme/read","permissions":{"pull":true}},{"full_name":"acme/unknown"},{"full_name":"acme/archived","archived":true,"permissions":{"push":true}}]`)
		} else {
			fmt.Fprint(w, `[{"full_name":"acme/another","permissions":{"push":true}},{"full_name":"acme/disabled","disabled":true,"permissions":{"push":true}}]`)
		}
	})
	rows, err := client.WritableRepositories(context.Background())
	if err != nil || !reflect.DeepEqual(rows, []string{"acme/another", "acme/write"}) {
		t.Fatalf("%v %v", rows, err)
	}
}
func TestCheckWritableRejectsUnknownAndLostPermission(t *testing.T) {
	for _, body := range []string{`{}`, `{"permissions":{"push":false}}`, `{"archived":true,"permissions":{"push":true}}`, `{"disabled":true,"permissions":{"push":true}}`} {
		client := fixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		if err := client.CheckWritable(context.Background(), "acme/repo"); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	client := fixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"permissions":{"push":true}}`) })
	if err := client.CheckWritable(context.Background(), "acme/repo"); err != nil {
		t.Fatal(err)
	}
}
