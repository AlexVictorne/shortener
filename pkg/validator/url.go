package validator

import (
	"errors"
	"net/url"
	"strings"
)

func ValidateURL(val string) (string, error) {
	if val == "" {
		return "", errors.New("URL is empty, cannot continue")
	}

	origVal := val

	if !strings.Contains(val, "://") {
		val = "http://" + val
	}

	u, err := url.Parse(val)
	if err != nil {
		return "", errors.New("URL is invalid ('" + origVal + "'), cannot continue")
	}

	scheme := u.Scheme
	host := u.Host
	hostname := u.Hostname()
	port := u.Port()

	if scheme == "" {
		return "", errors.New("URL: scheme is missing ('" + origVal + "'), cannot continue")
	}

	if host == "" {
		return "", errors.New("URL: host is empty or invalid ('" + host + "'), cannot continue")
	}

	if hostname == "" {
		return "", errors.New("URL: hostname is empty or invalid ('" + host + "'), cannot continue")
	}

	if port == "" {
		return "", errors.New("URL: port is missing, cannot continue")
	}

	return u.String(), nil
}
