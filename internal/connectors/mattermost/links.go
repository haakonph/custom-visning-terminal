package mattermost

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/haakonph/custom-visning-terminal/internal/model"
)

var prCandidate = regexp.MustCompile(`https://github\.com/[^\s<>()\[\]"'` + "`" + `]+`)
var repoPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// PRLinks returns canonical, unique PR URLs in first-mention order, including
// links to a PR's files, commits and discussion anchors.
func PRLinks(text string) []model.PRLink {
	var out []model.PRLink
	seen := map[string]bool{}
	for _, candidate := range prCandidate.FindAllString(text, -1) {
		u, err := url.Parse(strings.TrimRight(candidate, ".,;!"))
		if err != nil || u.Host != "github.com" || u.User != nil {
			continue
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 4 || len(parts) > 5 || parts[2] != "pull" || !repoPart.MatchString(parts[0]) || !repoPart.MatchString(parts[1]) {
			continue
		}
		if len(parts) == 5 && parts[4] != "files" && parts[4] != "commits" && parts[4] != "checks" {
			continue
		}
		if strings.Trim(parts[3], "0123456789") != "" {
			continue
		}
		n, err := strconv.Atoi(parts[3])
		if err != nil || n <= 0 {
			continue
		}
		owner, repo := strings.ToLower(parts[0]), strings.ToLower(parts[1])
		canonical := "https://github.com/" + owner + "/" + repo + "/pull/" + strconv.Itoa(n)
		if !seen[canonical] {
			seen[canonical] = true
			out = append(out, model.PRLink{Owner: owner, Repository: repo, Number: n, URL: canonical})
		}
	}
	return out
}
