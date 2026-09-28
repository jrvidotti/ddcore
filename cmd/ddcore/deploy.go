package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/scaffold"
)

const deployUsage = `usage: ddcore deploy <target>

  docker    Dockerfile (on the official image, pinned to this binary's release)
            and .dockerignore
  railway   the same, plus railway.json, and the variables to set on the service

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
		more, err = scaffold.Railway(dir)
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
		fmt.Print("\n" + scaffold.RailwayVariables)
	}
	return nil
}
