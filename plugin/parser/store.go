package parser

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func getConfigMapFromStore(ctx context.Context, kvc kvstore.Store, service string) (structs.ConfigsMap, error) {
	key := strings.Join([]string{configKey, service}, "/")

	pairs, err := kvc.ListKV(ctx, key)
	if err != nil {
		return nil, err
	}

	return parseListToConfigs(pairs)
}

func parseListToConfigs(pairs api.KVPairs) (structs.ConfigsMap, error) {
	cm := make(structs.ConfigsMap, len(pairs))

	for i := range pairs {
		c := structs.ConfigCmds{}
		err := json.Unmarshal(pairs[i].Value, &c)
		if err != nil {
			return cm, errors.WithStack(err)
		}

		cm[c.ID] = c
	}

	return cm, nil
}

func putConfigsToStore(ctx context.Context, client kvstore.Store, prefix string, configs structs.ConfigsMap) error {
	buf := bytes.NewBuffer(nil)

	for id, val := range configs {
		buf.Reset()

		err := json.NewEncoder(buf).Encode(val)
		if err != nil {
			return err
		}

		key := strings.Join([]string{configKey, prefix, id}, "/")
		err = client.PutKV(ctx, key, buf.Bytes())
		if err != nil {
			return err
		}
	}

	return nil
}

func getTemplateFromStore(ctx context.Context, kvc kvstore.Store, image, version string) (structs.ConfigTemplate, error) {
	t := structs.ConfigTemplate{}
	key := strings.Join([]string{imageKey, image, version}, "/")

	pair, err := kvc.GetKV(ctx, key)
	if err != nil {
		return t, err
	}

	if pair == nil || pair.Value == nil {
		return t, errors.Errorf("template:%s is not exist", key)
	}

	err = json.Unmarshal(pair.Value, &t)
	if err != nil {
		return t, errors.WithStack(err)
	}

	return t, nil
}

func putTemplateToStore(ctx context.Context, kvc kvstore.Store, t structs.ConfigTemplate) error {
	dat, err := json.Marshal(t)
	if err != nil {
		return errors.WithStack(err)
	}

	path := make([]string, 1, 3)
	path[0] = imageKey
	path = append(path, strings.SplitN(t.Image, ":", 2)...)

	key := strings.Join(path, "/")

	return kvc.PutKV(ctx, key, dat)
}
