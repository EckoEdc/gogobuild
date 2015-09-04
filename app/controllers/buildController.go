package controllers

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/revel/revel"
)

//BuildController Controller
type BuildController struct {
	*revel.Controller
}

//Index page
func (c BuildController) Index() revel.Result {
	builds, err := BMInstance().GetBuildsByProjects(c.Params.Get("project"))
	if err != nil {
		c.Flash.Error(err.Error())
	}
	if c.Params.Get("format") == "json" {
		return c.RenderJson(builds)
	}
	project := PMInstance().GetProjectByName(c.Params.Get("project"))
	project.Reload()
	return c.Render(builds, project)
}

//Detail of a build page
func (c BuildController) Detail() revel.Result {
	build, err := BMInstance().GetBuildByID(c.Params.Get("id"))
	if err != nil {
		c.Flash.Error(err.Error())
		return c.Render()
	}
	if c.Params.Get("format") == "json" {
		return c.RenderJson(build)
	}

	logsPath := fmt.Sprintf("%s/public/output/%s/%d/%s/logs.txt", revel.BasePath, build.ProjectToBuild.Name, build.Date.Unix(), build.TargetSys)
	logFile, err := ioutil.ReadFile(logsPath)
	if err != nil {
		c.Flash.Error(err.Error())
		revel.WARN.Println(err)
	}
	logContent := string(logFile)
	return c.Render(build, logContent)
}

//Retry a failed build
func (c BuildController) Retry() revel.Result {
	build, err := BMInstance().GetBuildByID(c.Params.Get("id"))
	if err != nil {
		c.Flash.Error(err.Error())
	}
	BMInstance().RetryBuild(build)
	return c.Redirect("/projects/%s/builds", build.ProjectToBuild.Name)
}

//Download the build result
func (c BuildController) Download() revel.Result {
	build, err := BMInstance().GetBuildByID(c.Params.Get("id"))
	if err != nil {
		revel.ERROR.Println(err)
		c.Flash.Error(err.Error())
	}
	if build.State > Fail {
		var outputAddr string
		if len(build.ProjectToBuild.Configuration.Package[build.TargetSys]) == 0 {
			//Test for tar archive or create it
			tarFile := fmt.Sprintf("/public/output/%s/%d/%s/output.tar", build.ProjectToBuild.Name, build.Date.Unix(), build.TargetSys)
			if _, err := os.Stat(revel.BasePath + tarFile); os.IsNotExist(err) {
				if err := build.CreateOutputTar(); err != nil {
					revel.ERROR.Println(err)
					c.Flash.Error(err.Error())
					return c.Redirect("/projects/%s/builds/%s", build.ProjectToBuild.Name, build.ID.Hex())
				}
			}
			outputAddr = tarFile
		} else {
			outputAddr = fmt.Sprintf("/public/output/%s/%d/%s/%s", build.ProjectToBuild.Name, build.Date.Unix(), build.TargetSys, build.ProjectToBuild.Configuration.Package[build.TargetSys])
		}
		if c.Params.Get("format") == "json" {
			return c.RenderJson(outputAddr)
		}
		return c.Redirect(outputAddr)
	}
	if c.Params.Get("format") == "json" {
		return c.RenderJson("")
	}
	return c.Redirect("/projects/%s/builds/%s", build.ProjectToBuild.Name, build.ID.Hex())
}

//Deploy the built package
func (c BuildController) Deploy() revel.Result {
	build, err := BMInstance().GetBuildByID(c.Params.Get("id"))
	if err != nil {
		revel.ERROR.Println(err)
		c.Flash.Error(err.Error())
	}
	if build.State > Fail {
		c.Flash.Success("Deploying %s for %s", build.ProjectToBuild.Name, build.TargetSys)
		go BMInstance().Deploy(build)
	}
	return c.Redirect("/projects/%s/builds/%s", build.ProjectToBuild.Name, build.ID.Hex())
}
