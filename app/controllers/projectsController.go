package controllers

import (
	"fmt"

	"github.com/revel/revel"
)

//ProjectsController struct
type ProjectsController struct {
	*revel.Controller
}

//Index Page
func (pc ProjectsController) Index() revel.Result {
	projectsList := PMInstance().GetProjectsList()
	if pc.Params.Get("format") == "json" {
		return pc.RenderJson(projectsList)
	}
	return pc.Render(projectsList)
}

//Build a project
func (pc ProjectsController) Build() revel.Result {
	var deploy bool
	if pc.Params.Get("commit") != "master" {
		pc.Flash.Error("Some men just want to see the world burn (Not Deploying non-master branch)")
		deploy = false
	} else {
		deploy = len(pc.Params.Get("submitDeploy")) > 0
	}
	build, _ := BMInstance().CreateOrReturnStatusBuild(pc.Params.Get("project"), pc.Params.Get("sys"), pc.Params.Get("commit"), deploy)
	if build.State == Created {
		pc.Flash.Success("Build Started %s %s", pc.Params.Get("project"), pc.Params.Get("sys"))
	} else if build.State == Fail {
		pc.Flash.Error(fmt.Sprintf("Build %s %s as state %s. Retrying...", pc.Params.Get("project"), pc.Params.Get("sys"), build.State.String()))
	} else {
		pc.Flash.Success("Build %s %s as state %s for refs %s. Nothing to do here.", pc.Params.Get("project"), pc.Params.Get("sys"), build.State.String(), pc.Params.Get("commit"))
	}
	if pc.Params.Get("format") == "json" {
		return pc.RenderJson(build)
	}
	return pc.Redirect("/projects/%s/builds", pc.Params.Get("project"))
}
