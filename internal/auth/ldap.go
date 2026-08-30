package auth

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/pepa/pepa/internal/config"
)

// LDAPUserInfo holds user attributes retrieved from LDAP.
type LDAPUserInfo struct {
	DN     string
	Email  string
	Name   string
	Groups []string
}

// LDAPProvider handles LDAP/Active Directory authentication.
type LDAPProvider struct {
	config config.LDAPConfig
}

// NewLDAPProvider creates a new LDAP authentication provider.
func NewLDAPProvider(cfg config.LDAPConfig) *LDAPProvider {
	return &LDAPProvider{config: cfg}
}

// TestConnection verifies that we can connect and bind to the LDAP server.
func (p *LDAPProvider) TestConnection(ctx context.Context) error {
	conn, err := p.connect()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Bind with service account
	if err := conn.Bind(p.config.BindDN, p.config.BindPassword); err != nil {
		return fmt.Errorf("bind with service account: %w", err)
	}

	return nil
}

// Authenticate performs LDAP authentication:
// 1. Connect to LDAP server
// 2. Bind with service account (BindDN)
// 3. Search for user by email using UserFilter
// 4. Attempt user bind with found DN + provided password
// 5. On success, fetch user attributes and group memberships
// 6. Return user info
func (p *LDAPProvider) Authenticate(ctx context.Context, email, password string) (*LDAPUserInfo, error) {
	conn, err := p.connect()
	if err != nil {
		return nil, fmt.Errorf("connect to LDAP: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Step 1: Bind with service account
	if p.config.BindDN != "" {
		if err := conn.Bind(p.config.BindDN, p.config.BindPassword); err != nil {
			return nil, fmt.Errorf("service account bind: %w", err)
		}
	}

	// Step 2: Search for user by email
	emailAttr := p.config.EmailAttr
	if emailAttr == "" {
		emailAttr = "mail"
	}
	userFilter := p.config.UserFilter
	if userFilter == "" {
		userFilter = "(&(objectClass=person)(" + emailAttr + "=%s))"
	}
	// The filter template must contain exactly one %s placeholder for the
	// email value. The email attribute name is configured via EmailAttr.
	filter := fmt.Sprintf(userFilter, ldap.EscapeFilter(email))

	nameAttr := p.config.NameAttr
	if nameAttr == "" {
		nameAttr = "cn"
	}

	searchReq := &ldap.SearchRequest{
		BaseDN:       p.config.BaseDN,
		Scope:        ldap.ScopeWholeSubtree,
		DerefAliases: ldap.NeverDerefAliases,
		SizeLimit:    1,
		TimeLimit:    10,
		TypesOnly:    false,
		Filter:       filter,
		Attributes:   []string{emailAttr, nameAttr, "dn", "memberOf", "member"},
	}

	sr, err := conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("search for user: %w", err)
	}
	if len(sr.Entries) == 0 {
		return nil, fmt.Errorf("user not found in LDAP")
	}
	if len(sr.Entries) > 1 {
		return nil, fmt.Errorf("multiple users found in LDAP")
	}

	entry := sr.Entries[0]
	userDN := entry.DN
	userEmail := entry.GetAttributeValue(emailAttr)
	userName := entry.GetAttributeValue(nameAttr)

	// Step 3: Attempt user bind to verify password
	if err := conn.Bind(userDN, password); err != nil {
		return nil, fmt.Errorf("user bind failed (invalid credentials): %w", err)
	}

	// Step 4: Fetch group memberships (deduplicated)
	groupSet := make(map[string]bool)
	for _, g := range p.extractGroups(entry) {
		groupSet[g] = true
	}

	// If group filter is set, also search for groups the user belongs to
	if p.config.GroupFilter != "" {
		directGroups, err := p.searchGroups(conn, userDN)
		if err == nil {
			for _, g := range directGroups {
				groupSet[g] = true
			}
		}
	}

	groups := make([]string, 0, len(groupSet))
	for g := range groupSet {
		groups = append(groups, g)
	}

	return &LDAPUserInfo{
		DN:     userDN,
		Email:  userEmail,
		Name:   userName,
		Groups: groups,
	}, nil
}

// connect establishes a connection to the LDAP server.
func (p *LDAPProvider) connect() (*ldap.Conn, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: p.config.InsecureSkipVerify, //nolint:gosec // configurable for self-signed certs
	}

	dialOpts := []ldap.DialOpt{
		ldap.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}),
	}

	if strings.HasPrefix(p.config.URL, "ldaps://") {
		dialOpts = append(dialOpts, ldap.DialWithTLSConfig(tlsConfig))
		return ldap.DialURL(p.config.URL, dialOpts...)
	}

	conn, err := ldap.DialURL(p.config.URL, dialOpts...)
	if err != nil {
		return nil, err
	}

	if p.config.StartTLS {
		if err := conn.StartTLS(tlsConfig); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("StartTLS: %w", err)
		}
	}

	return conn, nil
}

// extractGroups extracts group memberships from the LDAP entry's memberOf attribute.
func (p *LDAPProvider) extractGroups(entry *ldap.Entry) []string {
	return entry.GetAttributeValues("memberOf")
}

// searchGroups searches for groups that contain the user as a member.
func (p *LDAPProvider) searchGroups(conn *ldap.Conn, userDN string) ([]string, error) {
	filter := fmt.Sprintf(p.config.GroupFilter, ldap.EscapeFilter(userDN))
	searchReq := &ldap.SearchRequest{
		BaseDN:       p.config.BaseDN,
		Scope:        ldap.ScopeWholeSubtree,
		DerefAliases: ldap.NeverDerefAliases,
		SizeLimit:    100,
		TimeLimit:    10,
		TypesOnly:    false,
		Filter:       filter,
		Attributes:   []string{"dn", "cn"},
	}

	sr, err := conn.Search(searchReq)
	if err != nil {
		return nil, err
	}

	var groups []string
	for _, entry := range sr.Entries {
		groups = append(groups, entry.DN)
	}
	return groups, nil
}

// MapGroupsToRoles maps LDAP groups to PEPA roles using the configured group mapping.
func (p *LDAPProvider) MapGroupsToRoles(groups []string) []string {
	if p.config.GroupMapping == nil || len(groups) == 0 {
		return nil
	}

	var roles []string
	roleSet := make(map[string]bool)
	for _, group := range groups {
		if role, ok := p.config.GroupMapping[group]; ok && !roleSet[role] {
			roles = append(roles, role)
			roleSet[role] = true
		}
		// Also try case-insensitive matching
		groupLower := strings.ToLower(group)
		for mappedGroup, role := range p.config.GroupMapping {
			if strings.ToLower(mappedGroup) == groupLower && !roleSet[role] {
				roles = append(roles, role)
				roleSet[role] = true
			}
		}
	}
	return roles
}
