package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"strings"
	"sync"

	"aetrna-music/locales"
)

type Manager struct {
	locales     map[string]map[string]string
	defaultLang string
	mu          sync.RWMutex
}

var Globali18n = NewManager("en")

func NewManager(defaultLang string) *Manager {
	m := &Manager{
		locales:     make(map[string]map[string]string),
		defaultLang: defaultLang,
	}
	_ = m.LoadEmbeddedLocales()
	return m
}

func (m *Manager) LoadEmbeddedLocales() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := fs.ReadDir(locales.FS, ".")
	if err != nil {
		log.Printf("⚠️ [i18n] Error reading embedded locales: %v", err)
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		lang := strings.TrimSuffix(entry.Name(), ".json")
		data, err := locales.FS.ReadFile(entry.Name())
		if err != nil {
			log.Printf("⚠️ [i18n] Failed to read embedded locale %s: %v", entry.Name(), err)
			continue
		}

		var translations map[string]string
		if err := json.Unmarshal(data, &translations); err != nil {
			log.Printf("⚠️ [i18n] Failed to parse JSON in embedded locale %s: %v", entry.Name(), err)
			continue
		}

		m.locales[lang] = translations
		log.Printf("🌐 [i18n] Loaded embedded locale '%s' (%d keys)", lang, len(translations))
	}
	return nil
}

func (m *Manager) T(lang, key string, args ...interface{}) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if lang == "" {
		lang = m.defaultLang
	}

	dict, exists := m.locales[lang]
	if !exists {
		dict = m.locales[m.defaultLang]
	}

	val, found := dict[key]
	if !found {
		if defaultDict, ok := m.locales[m.defaultLang]; ok {
			val = defaultDict[key]
		}
	}

	if val == "" {
		val = key
	}

	if len(args) > 0 {
		return fmt.Sprintf(val, args...)
	}
	return val
}

func (m *Manager) AvailableLanguages() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make(map[string]string)
	for lang, dict := range m.locales {
		name := dict["lang_name"]
		if name == "" {
			name = lang
		}
		res[lang] = name
	}
	return res
}
