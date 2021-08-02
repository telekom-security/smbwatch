package main

import (
	"crypto/tls"

	"github.com/go-ldap/ldap/v3"
)

func getLdapServers(url, dn, pass, baseDn, filter string) ([]string, error) {
	var serverlist []string

	l, err := ldap.DialURL(url, ldap.DialWithTLSConfig(&tls.Config{InsecureSkipVerify: true}))
	if err != nil {
		return serverlist, err
	}
	defer l.Close()

	err = l.Bind(dn, pass)
	if err != nil {
		return serverlist, err
	}

	searchReq := ldap.NewSearchRequest(baseDn, ldap.ScopeWholeSubtree, 0, 0, 0, false, filter, []string{"DNSHostName"}, []ldap.Control{})

	result, err := l.SearchWithPaging(searchReq, 50)
	if err != nil {
		return serverlist, err
	}

	for _, r := range result.Entries {
		dns := r.GetAttributeValue("dNSHostName")
		if dns == "" {
			continue
		}

		serverlist = append(serverlist, dns)
	}

	return serverlist, nil
}
