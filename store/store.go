package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

const storeFile = "roles.json"

type data struct {
	MessageID string            `json:"message_id"`
	Roles     map[string]string `json:"roles"` // emoji → role_id
}

type RoleStore struct {
	mu sync.RWMutex
	d  data
}

func Load() (*RoleStore, error) {
	s := &RoleStore{}

	bytes, err := os.ReadFile(storeFile)
	if os.IsNotExist(err) {
		s.d = data{Roles: make(map[string]string)}
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", storeFile, err)
	}

	if err := json.Unmarshal(bytes, &s.d); err != nil {
		return nil, fmt.Errorf("%s is corrupted: %w", storeFile, err)
	}

	if s.d.Roles == nil {
		s.d.Roles = make(map[string]string)
	}

	return s, nil
}

func (s *RoleStore) save() error {
	bytes, err := json.MarshalIndent(s.d, "", "  ")
	if err != nil {
		return err
	}
	tmp := storeFile + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, storeFile)
}

func (s *RoleStore) MessageID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.d.MessageID
}

func (s *RoleStore) SetMessageID(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d.MessageID = id
	return s.save()
}

func (s *RoleStore) Roles() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copy := make(map[string]string, len(s.d.Roles))
	for k, v := range s.d.Roles {
		copy[k] = v
	}
	return copy
}

func (s *RoleStore) RoleForEmoji(emoji string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roleID, ok := s.d.Roles[emoji]
	return roleID, ok
}

func (s *RoleStore) Add(emoji, roleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.d.Roles[emoji]; exists {
		return fmt.Errorf("emoji %s is already mapped to a role", emoji)
	}
	for e, r := range s.d.Roles {
		if r == roleID {
			return fmt.Errorf("role is already mapped to emoji %s", e)
		}
	}

	s.d.Roles[emoji] = roleID
	if err := s.save(); err != nil {
		delete(s.d.Roles, emoji)
		return fmt.Errorf("failed to save: %w", err)
	}
	return nil
}

func (s *RoleStore) SetMappings(roles map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.d.Roles
	s.d.Roles = roles
	if err := s.save(); err != nil {
		s.d.Roles = old
		return fmt.Errorf("failed to save mappings: %w", err)
	}
	return nil
}

func (s *RoleStore) Remove(emoji string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.d.Roles[emoji]; !exists {
		return fmt.Errorf("emoji %s is not mapped to any role", emoji)
	}

	roleID := s.d.Roles[emoji]
	delete(s.d.Roles, emoji)
	if err := s.save(); err != nil {
		s.d.Roles[emoji] = roleID
		return fmt.Errorf("failed to save: %w", err)
	}
	return nil
}
