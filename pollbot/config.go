package main

import (
	"net/url"
	"os"

	_ "github.com/joho/godotenv/autoload"
)

type config struct {
	mattermostUserName string
	mattermostTeamName string
	mattermostToken    string
	mattermostChannel  string
	mattermostServer   *url.URL
	tarantoolAddress   string
	tarantoolUser      string
	tarantoolPassword  string
}

func loadConfig() config {
	var settings config

	settings.mattermostTeamName = os.Getenv("MM_TEAM")
	settings.mattermostUserName = os.Getenv("MM_USERNAME")
	settings.mattermostToken = os.Getenv("MM_TOKEN")
	settings.mattermostChannel = os.Getenv("MM_CHANNEL")
	settings.mattermostServer, _ = url.Parse(os.Getenv("MM_SERVER"))
	settings.tarantoolAddress = os.Getenv("TT_SERVER")
	settings.tarantoolUser = os.Getenv("TT_USERNAME")
	settings.tarantoolPassword = os.Getenv("TT_PASSWORD")

	return settings
}
