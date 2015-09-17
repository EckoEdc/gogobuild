package controllers

import (
	"fmt"
	"log"

	"golang.org/x/build/gerrit"
)

//GerritManager for fetching changes
type GerritManager struct {
	gerritClient *gerrit.Client
	project      string
}

//Init function
func (g *GerritManager) Init(p *Project) {
	g.gerritClient = gerrit.NewClient(p.Configuration.ReviewAddress, nil)
	g.project = p.Name
}

//GetOpenChanges return open gerrit patchset for project
//TODO: Use the Mergable and title property ??
func (g *GerritManager) GetOpenChanges() ([]string, error) {
	query := fmt.Sprintf("project:%s status:open", g.project)
	changes, err := g.gerritClient.QueryChanges(query, gerrit.QueryChangesOpt{N: 0, Fields: []string{"ALL_REVISIONS"}})
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	var changeNumbers []string
	for _, change := range changes {
		changeNumbers = append(changeNumbers, fmt.Sprintf("refs/changes/%02d/%d/%d", change.ChangeNumber%100, change.ChangeNumber, len(change.Revisions)))
	}
	return changeNumbers, nil
}
