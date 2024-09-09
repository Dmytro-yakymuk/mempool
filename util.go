package main

import (
	"net/url"
	"strconv"

	"github.com/jackc/pgx"
)

// ParsePgConn parses a Postgres connection URL into pgx.ConnConfig.
func ParsePgConn(pgurl string) (*pgx.ConnConfig, error) {
	u, err := url.Parse(pgurl)
	if err != nil {
		return nil, err
	}

	user := u.User.Username()
	password, _ := u.User.Password()

	port, err := strconv.ParseUint(u.Port(), 10, 16)
	if err != nil {
		return nil, err
	}

	config := &pgx.ConnConfig{
		Host:     u.Hostname(),
		Port:     uint16(port),
		Database: u.Path[1:], // Remove leading '/' from the path.
		User:     user,
		Password: password,
	}

	return config, nil
}
