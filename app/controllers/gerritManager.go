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
	changes, err := g.gerritClient.QueryChanges(query, gerrit.QueryChangesOpt{N: 0, Fields: []string{"current_revision"}})
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	var changeNumbers []string
	for _, change := range changes {
		changeNumbers = append(changeNumbers, fmt.Sprintf("%s", change.Revisions[change.CurrentRevision].Ref))
	}
	return changeNumbers, nil
}
