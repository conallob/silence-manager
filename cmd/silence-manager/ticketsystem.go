package main

import (
	"fmt"
	"log"

	"github.com/conallob/silence-manager/pkg/config"
	"github.com/conallob/silence-manager/pkg/ticket"
)

// newTicketSystem builds the ticket system client selected by cfg.TicketSystem
func newTicketSystem(cfg *config.Config) (ticket.TicketSystem, error) {
	switch cfg.TicketSystem {
	case config.TicketSystemJira:
		log.Printf("Jira URL: %s, project: %s", cfg.Jira.URL, cfg.Jira.ProjectKey)
		return ticket.NewJiraTicketSystem(
			cfg.Jira.URL,
			cfg.Jira.Username,
			cfg.Jira.APIToken,
			cfg.Jira.ProjectKey,
			cfg.Sync.AnnotationPrefix,
		), nil
	case config.TicketSystemLinear:
		if cfg.Linear.OAuthToken != "" {
			log.Printf("Linear team: %s (OAuth access token)", cfg.Linear.TeamID)
			return ticket.NewLinearOAuthTicketSystem(
				cfg.Linear.APIURL,
				cfg.Linear.OAuthToken,
				cfg.Linear.TeamID,
				cfg.Sync.AnnotationPrefix,
			), nil
		}
		log.Printf("Linear team: %s (personal API key)", cfg.Linear.TeamID)
		return ticket.NewLinearTicketSystem(
			cfg.Linear.APIURL,
			cfg.Linear.APIKey,
			cfg.Linear.TeamID,
			cfg.Sync.AnnotationPrefix,
		), nil
	case config.TicketSystemPylon:
		log.Println("Warning: the Pylon integration is experimental and has not been verified against a live workspace")
		return ticket.NewPylonTicketSystem(
			cfg.Pylon.APIURL,
			cfg.Pylon.APIToken,
			cfg.Pylon.AccountID,
			cfg.Pylon.RequesterEmail,
			cfg.Sync.AnnotationPrefix,
		), nil
	default:
		return nil, fmt.Errorf("unknown ticket system: %q", cfg.TicketSystem)
	}
}
