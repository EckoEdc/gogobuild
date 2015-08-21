package controllers

import (
	"errors"

	"github.com/revel/modules/jobs/app/jobs"
)

//Worker interface
type Worker interface {
	Run()
}

//WorkerManager singleton
type WorkerManager struct {
}

//instance of WorkerManager
var instance *WorkerManager

//WMInstance Return the instance
func WMInstance() *WorkerManager {
	if instance == nil {
		instance = new(WorkerManager)
	}
	return instance
}

//Build Launch a build
//Build queue is restricted by jobs.pool = 4 in app.conf (FIFO)
func (w *WorkerManager) Build(build *Build) error {

	var launchFunc func(build *Build, targetSys string) Worker

	switch build.ProjectToBuild.Configuration.BuildType {
	case "Docker":
		launchFunc = w.launchDockerBuild
	default:
		build.State = Fail
		return errors.New("Not a valid build type")
	}
	jobs.Now(launchFunc(build, build.TargetSys))
	return nil
}

func (w *WorkerManager) launchDockerBuild(build *Build, targetSys string) Worker {
	d := DockerWorker{
		build:     *build,
		targetSys: targetSys,
	}
	return d
}
