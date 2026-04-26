package config

import (
	"bufio"
	"os"
	"strings"
)

// VIPList manages the list of VIP senders
// Can be loaded from env var (comma-separated) or a file (one per line)
type VIPList struct {
	senders map[string]bool
}

// LoadVIPList builds a VIPList from either a file path or comma-separated env string
func LoadVIPList(cfg Config) *VIPList {
	vl := &VIPList{senders: make(map[string]bool)}

	// Try file first (VIP_SENDERS_FILE=/path/to/vip.txt)
	if cfg.VIPSendersFile != "" {
		if err := vl.loadFromFile(cfg.VIPSendersFile); err == nil {
			return vl
		}
	}

	// Fall back to comma-separated env var (VIP_SENDERS=boss@company.com,cto@company.com)
	if cfg.VIPSenders != "" {
		for _, s := range strings.Split(cfg.VIPSenders, ",") {
			s = strings.TrimSpace(strings.ToLower(s))
			if s != "" {
				vl.senders[s] = true
			}
		}
	}

	return vl
}

// IsVIP returns true if the email address matches a VIP sender
// Supports exact match (user@domain.com) and domain match (@domain.com)
func (v *VIPList) IsVIP(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))

	// Extract just the email address from "Name <email@domain.com>" format
	if start := strings.Index(email, "<"); start != -1 {
		if end := strings.Index(email, ">"); end > start {
			email = email[start+1 : end]
		}
	}

	// Exact match
	if v.senders[email] {
		return true
	}

	// Domain match — if VIP list has "@domain.com" entry
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 2 {
		domain := "@" + parts[1]
		if v.senders[domain] {
			return true
		}
	}

	return false
}

// Add adds a sender to the VIP list at runtime
func (v *VIPList) Add(sender string) {
	v.senders[strings.ToLower(strings.TrimSpace(sender))] = true
}

// List returns all VIP senders
func (v *VIPList) List() []string {
	result := make([]string, 0, len(v.senders))
	for s := range v.senders {
		result = append(result, s)
	}
	return result
}

func (v *VIPList) loadFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		v.senders[strings.ToLower(line)] = true
	}
	return scanner.Err()
}