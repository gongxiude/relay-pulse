package newapi

import (
	"context"

	"monitor/internal/audit"
)

type ChannelLister interface {
	ListChannels(context.Context) (*ChannelList, error)
}

func SyncChannels(ctx context.Context, client ChannelLister, store audit.TargetStore) ([]string, error) {
	if client == nil {
		return nil, context.Canceled
	}
	res, err := client.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	specs := make([]audit.ChannelSpec, 0, len(res.Items))
	for _, ch := range res.Items {
		specs = append(specs, audit.ChannelSpec{
			ID:           ch.ID,
			Type:         ch.Type,
			Status:       ch.Status,
			Name:         ch.Name,
			BaseURL:      ch.BaseURL,
			Models:       ch.Models,
			Group:        ch.Group,
			Weight:       ch.Weight,
			Priority:     ch.Priority,
			ModelMapping: ch.ModelMapping,
			Other:        ch.Other,
		})
	}
	targets, err := audit.SyncTargets(ctx, specs, store)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		out = append(out, target.Provider+"/"+target.Channel+"/"+target.Model)
	}
	return out, nil
}
