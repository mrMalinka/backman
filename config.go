package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mcuadros/go-defaults"
)

var config Config

type Config struct {
	ArchiveDir string `json:"archive_dir"`
	TimeFormat string `json:"time_format" default:"02.01.2006 15:04:05"`
}

func (c *Config) SetDefaultDir() {
	c.ArchiveDir = getAppDir()
}

func loadConfig() {
	defaults.SetDefaults(&config)
	config.SetDefaultDir()

	path := getConfigPath()

	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// defaults have been loaded
		return
	}

	err = json.Unmarshal(contents, &config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error parsing config: ", err)
		os.Exit(1)
	}
}

const appName = "backman"

func resolveDir(xdgEnv, winSubpath, unixSubpath string) string {
	if dir := os.Getenv(xdgEnv); dir != "" {
		return dir
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("AppData"); appData != "" {
			return filepath.Join(appData, winSubpath)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			panic(fmt.Errorf("cannot determine user home directory: %w", err))
		}
		return filepath.Join(home, "AppData", "Roaming", winSubpath)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("cannot determine user home directory: %w", err))
	}
	return filepath.Join(home, unixSubpath)
}

func getAppDir() string {
	// XDG_DATA_HOME, fallback to Windows/AppData/<appName> or ~/.local/share/<appName>
	base := resolveDir("XDG_DATA_HOME", appName, filepath.Join(".local", "share", appName))
	return base
}

func getConfigPath() string {
	// XDG_CONFIG_HOME, fallback to Windows/AppData/<appName> or ~/.config/<appName>/<appName>.json
	dir := resolveDir("XDG_CONFIG_HOME", appName, filepath.Join(".config", appName))
	return filepath.Join(dir, appName+".json")
}
