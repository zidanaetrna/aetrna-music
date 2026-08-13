package i18n

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Manager struct {
	locales     map[string]map[string]string
	defaultLang string
	mu          sync.RWMutex
}

var Globali18n = NewManager("./locales", "en")

func NewManager(localesDir, defaultLang string) *Manager {
	m := &Manager{
		locales:     make(map[string]map[string]string),
		defaultLang: defaultLang,
	}
	_ = m.LoadLocales(localesDir)
	return m
}

func (m *Manager) LoadLocales(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("⚠️ [i18n] Warning reading locales dir %q: %v", dir, err)
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		lang := strings.TrimSuffix(entry.Name(), ".json")
		filePath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("⚠️ [i18n] Failed to read locale file %s: %v", filePath, err)
			continue
		}

		var translations map[string]string
		if err := json.Unmarshal(data, &translations); err != nil {
			log.Printf("⚠️ [i18n] Failed to parse JSON in %s: %v", filePath, err)
			continue
		}

		m.locales[lang] = translations
		log.Printf("🌐 [i18n] Loaded locale '%s' (%d keys)", lang, len(translations))
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
