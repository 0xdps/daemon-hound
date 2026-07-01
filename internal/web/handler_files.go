package web

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/0xdps/daemon-hound/internal/models"
)

type namespace struct {
	Name  string
	Files []models.TrackedFile
}

type fileListData struct {
	Namespaces []namespace
	Total      int
	Filtered   int
	Query      string
}

type fileViewData struct {
	File    models.TrackedFile
	Content string
	Error   string
}

func (s *Server) handleFileList(w http.ResponseWriter, r *http.Request) {
	state, err := s.vault.LoadState()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	needle := strings.ToLower(query)

	nsMap := map[string]*namespace{}
	for _, f := range state.Files {
		if needle != "" {
			haystack := strings.ToLower(f.Namespace + ":" + f.RelPath + " " + fmt.Sprint(f.Mode))
			if !strings.Contains(haystack, needle) {
				continue
			}
		}
		ns := f.Namespace
		if _, ok := nsMap[ns]; !ok {
			nsMap[ns] = &namespace{Name: ns}
		}
		nsMap[ns].Files = append(nsMap[ns].Files, f)
	}

	var nsList []namespace
	for _, ns := range nsMap {
		sort.Slice(ns.Files, func(i, j int) bool {
			return ns.Files[i].RelPath < ns.Files[j].RelPath
		})
		nsList = append(nsList, *ns)
	}
	sort.Slice(nsList, func(i, j int) bool { return nsList[i].Name < nsList[j].Name })

	filtered := 0
	for _, ns := range nsList {
		filtered += len(ns.Files)
	}

	data := fileListData{Namespaces: nsList, Total: len(state.Files), Filtered: filtered, Query: query}
	if isHTMX(r) {
		s.renderPartial(w, "files/list.html", data)
		return
	}
	s.render(w, "files", "files/list.html", data)
}

func (s *Server) handleFileView(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key") // "namespace:relPath"
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		http.Error(w, "key parameter must be namespace:relPath", 400)
		return
	}

	state, err := s.vault.LoadState()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	file, ok := state.Files[key]
	if !ok {
		http.Error(w, fmt.Sprintf("file not found: %s", key), 404)
		return
	}

	data := fileViewData{File: file}
	plaintext, err := s.vault.RetrieveFile(file)
	if err != nil {
		data.Error = err.Error()
	} else {
		data.Content = string(plaintext)
	}

	if isHTMX(r) {
		s.renderPartial(w, "files/view.html", data)
		return
	}
	s.render(w, "files", "files/view.html", data)
}
