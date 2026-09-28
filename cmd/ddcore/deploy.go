package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/scaffold"
)

const deployUsage = `usage: ddcore deploy <target>

  docker    Dockerfile (on the official image, pinned to this binary's release)
            and .dockerignore
  railway   the same, plus .railway/railway.ts — the Railway project as code (service,
            PostgreSQL, health check), applied with railway config plan / apply

An existing file is left alone. Run it next to ddcore.json.
`

// cmdDeploy writes the files a deployment builds from. It never touches a
// file that exists: a site's Dockerfile is its own once written.
func cmdDeploy(args []string) error {
	if len(args) != 1 || (args[0] != "docker" && args[0] != "railway") {
		return fmt.Errorf("%s", deployUsage)
	}
	_, path, err := config.Load(".")
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s not found: run `ddcore deploy` in a site (or `ddcore init` first)", config.Name)
	}
	dir := filepath.Dir(path)
	tag, release := engine.ImageTag(engine.Version)
	files, err := scaffold.Docker(dir, tag)
	if err == nil && args[0] == "railway" {
		var more []string
		more, err = scaffold.Railway(dir, railwaySite(dir))
		files = append(files, more...)
	}
	for _, f := range files {
		fmt.Println("created", f)
	}
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("nothing to do: every file already exists")
	}
	if !release {
		fmt.Printf("\nthis binary is not a release (%s): the Dockerfile uses %s:latest — pin a version before production\n", engine.Version, scaffold.Image)
	}
	if args[0] == "railway" {
		if _, err := os.Stat(filepath.Join(dir, "railway.json")); err == nil {
			fmt.Println("\nrailway.json is Railway's legacy format, read only until 2026-12-01: run")
			fmt.Println("`railway config migrate --apply --delete-files` to retire it, then merge what it")
			fmt.Println("printed into .railway/railway.ts")
		}
		fmt.Print("\n" + scaffold.RailwayNext)
	}
	return nil
}

// railwaySite names the Railway project after the site's directory and finds
// the GitHub repository and branch a push to which should deploy, from the
// checkout's origin. Outside a GitHub checkout the source is left for the
// operator to write.
func railwaySite(dir string) scaffold.RailwaySite {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	site := scaffold.RailwaySite{Name: filepath.Base(abs)}
	if out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output(); err == nil {
		site.Repo = githubRepo(strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "HEAD").Output(); err == nil {
		site.Branch = strings.TrimSpace(string(out))
	}
	return site
}

// githubRepo reads owner/repo from a GitHub remote, SSH or HTTPS; anything
// else is not a source Railway's github() takes, and gives "".
func githubRepo(url string) string {
	for _, p := range []string{"git@github.com:", "ssh://git@github.com/", "https://github.com/", "http://github.com/"} {
		if strings.HasPrefix(url, p) {
			repo := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(url, p), "/"), ".git")
			if strings.Count(repo, "/") == 1 {
				return repo
			}
		}
	}
	return ""
}
