package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"

	"github.com/apolloconfig/agollo/v4"
	"github.com/apolloconfig/agollo/v4/env/config"
	"github.com/apolloconfig/agollo/v4/storage"
)

func splitKey(key string) (string, string) {
	namespace := "default"
	idx := strings.IndexByte(key, ':')
	if idx != -1 {
		if idx > 0 {
			namespace = key[:idx]
		}
		key = key[idx+1:]
	}
	if key == "" {
		key = "default"
	}
	return namespace, key
}

func joinKey(namespace, key string) string {
	return namespace + ":" + key
}

func isDataNamespace(namespace string) bool {
	return strings.HasSuffix(namespace, ".json")
}

func fileSha256(filePath string) ([]byte, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte{}, nil
		}
		return nil, err
	}
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil), nil
}

type ApolloConfd struct {
	client         agollo.Client
	lock           sync.Mutex
	tpl            *template.Template
	data           map[string]interface{}
	namespaces     []string
	watched        int32
	watchGroups    []watchGroup
	fileWatchGroup map[string][]int
}

type watchGroup struct {
	path     string
	onChange string
}

func NewApolloConfd(conf *ApolloConfdConfig) (*ApolloConfd, error) {
	c := &config.AppConfig{
		AppID:         conf.Apollo.AppID,
		Cluster:       conf.Apollo.Cluster,
		NamespaceName: strings.Join(conf.Apollo.Namespaces, ","),
		IP:            conf.Apollo.API,
		Secret:        conf.Apollo.Secret,
	}
	client, err := agollo.StartWithConfig(func() (*config.AppConfig, error) {
		return c, nil
	})
	if err != nil {
		return nil, err
	}
	confd := &ApolloConfd{
		client:         client,
		data:           make(map[string]interface{}),
		namespaces:     conf.Apollo.Namespaces,
		tpl:            template.New("apollo-confd"),
		fileWatchGroup: make(map[string][]int),
	}
	for _, watch := range conf.Watch {
		confd.InitWatch(watch)
	}
	for _, ns := range confd.namespaces {
		err := confd.InitNamespace(ns)
		if err != nil {
			return nil, err
		}
	}
	return confd, nil
}

func (c *ApolloConfd) InitWatch(config WatchConfig) {
	for _, group := range config.Groups {
		for _, fullKey := range group.Keys {
			c.lock.Lock()
			if _, ok := c.fileWatchGroup[fullKey]; !ok {
				c.fileWatchGroup[fullKey] = make([]int, 0, 10)
			}
			c.fileWatchGroup[fullKey] = append(c.fileWatchGroup[fullKey], len(c.watchGroups))
			c.watchGroups = append(c.watchGroups, watchGroup{
				path:     group.Path,
				onChange: config.OnChange,
			})
			c.lock.Unlock()
		}
	}
}

func (c *ApolloConfd) InitNamespace(namespace string) error {
	cache := c.client.GetConfigCache(namespace)
	if isDataNamespace(namespace) {
		v, err := cache.Get("content")
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(namespace, ".json")
		return c.saveJSONData(name, v.(string))
	} else {
		cache.Range(func(key, val interface{}) bool {
			fullKey := joinKey(namespace, key.(string))
			err := c.loadTemplate(fullKey, val.(string))
			if err != nil {
				log.Printf("[ERROR] load tempate of config %s with error: %s", key, err)
			}
			return true
		})
	}
	return nil
}

func (c *ApolloConfd) loadTemplate(fullKey, text string) error {
	_, err := c.tpl.New(fullKey).Parse(text)
	return err
}

func (c *ApolloConfd) removeData(name string) {
	c.lock.Lock()
	delete(c.data, name)
	c.lock.Unlock()
}

func (c *ApolloConfd) saveJSONData(name, jsonStr string) error {
	data := make(map[string]interface{})
	err := json.Unmarshal([]byte(jsonStr), &data)
	if err != nil {
		return err
	}
	c.lock.Lock()
	c.data[name] = data
	c.lock.Unlock()
	return nil
}

func (c *ApolloConfd) renderAndSave(fullKey, target string) (bool, error) {
	err := os.MkdirAll(target, 0755)
	if err != nil {
		return false, err
	}
	_, key := splitKey(fullKey)
	filePath := path.Join(target, key)
	before, err := fileSha256(filePath)
	if err != nil {
		return false, err
	}
	newPath := filePath + ".inprocess"
	f, err := os.OpenFile(newPath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	err = c.tpl.ExecuteTemplate(f, fullKey, c.data)
	if err != nil {
		return false, err
	}
	err = os.Rename(newPath, filePath)
	if err != nil {
		return false, err
	}
	after, err := fileSha256(filePath)
	if err != nil {
		return false, err
	}
	return !bytes.Equal(before, after), nil
}

func (c *ApolloConfd) LoadAll() (bool, error) {
	var changed bool
	for fullKey, groupIndices := range c.fileWatchGroup {
		namespace, key := splitKey(fullKey)
		for _, index := range groupIndices {
			group := c.watchGroups[index]
			v, err := c.renderAndSave(fullKey, group.path)
			if err != nil {
				log.Printf("[ERROR] render file %s/%s of namespace %s with error: %s", group.path, key, namespace, err)
				continue
			}
			if v {
				changed = true
			}
			log.Printf("[INFO] render file %s/%s of namespace %s, file changed: %t", group.path, key, namespace, v)
		}
	}
	return changed, nil
}

func (c *ApolloConfd) LoadAndWatch() (bool, error) {
	if atomic.CompareAndSwapInt32(&c.watched, 0, 1) {
		c.client.AddChangeListener(c)
	}
	return c.LoadAll()
}

func (c *ApolloConfd) OnChange(event *storage.ChangeEvent) {
	var (
		dataChanged   bool
		changedGroups = make(map[int]watchGroup)
	)
	for key, change := range event.Changes {
		fullKey := joinKey(event.Namespace, key)
		groupIndices, ok := c.fileWatchGroup[fullKey]
		switch change.ChangeType {
		case storage.DELETED:
			if isDataNamespace(event.Namespace) {
				if len(event.Changes) == 0 {
					return
				}
				name := strings.TrimSuffix(event.Namespace, ".json")
				log.Printf("[INFO] remove data of namespace %s on change", event.Namespace)
				c.removeData(name)
				dataChanged = true
			} else if ok {
				for _, index := range groupIndices {
					group := c.watchGroups[index]
					filePath := path.Join(group.path, key)
					log.Printf("[INFO] remove local file %s on change", filePath)
					err := os.Remove(filePath)
					if err != nil {
						log.Printf("[ERROR] remove local file %s on change with error: %s", filePath, err)
					}
					changedGroups[index] = group
				}
			}
		case storage.ADDED, storage.MODIFIED:
			if isDataNamespace(event.Namespace) {
				log.Printf("[INFO] init data namespace %s on change", event.Namespace)
				c.InitNamespace(event.Namespace)
				dataChanged = true
			} else if ok {
				fullKey := joinKey(event.Namespace, key)
				log.Printf("[INFO] load template %s on change", fullKey)
				err := c.loadTemplate(fullKey, change.NewValue.(string))
				if err != nil {
					log.Printf("[ERROR] load template of config %s on change with error: %s", fullKey, err)
				}
				for _, index := range groupIndices {
					group := c.watchGroups[index]
					log.Printf("[INFO] render config %s of %s on change", path.Join(group.path, key), fullKey)
					changed, err := c.renderAndSave(fullKey, group.path)
					if err != nil {
						log.Printf("[ERROR] render config %s on change with error: %s", fullKey, err)
					}
					if changed {
						changedGroups[index] = group
					}
				}
			}
		default:
			continue
		}
	}
	if dataChanged {
		log.Printf("[INFO] data changed, reload all config")
		changed, err := c.LoadAll()
		if err != nil {
			log.Printf("[ERROR] render all config files with error: %s", err)
		}
		if changed {
			for i, group := range c.watchGroups {
				changedGroups[i] = group
			}
		}
	}
	for _, group := range changedGroups {
		if group.onChange != "" {
			log.Printf("[INFO] file changed, exec %s", group.onChange)
			err := exec.Command(group.onChange).Start()
			if err != nil {
				log.Printf("[ERROR] execute onchange command %s with error: %s", group.onChange, err)
			}
		}
	}
}

func (c *ApolloConfd) OnNewestChange(event *storage.FullChangeEvent) {}
