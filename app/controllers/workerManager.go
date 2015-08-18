package controllers

import "errors"

//Worker interface
type Worker interface {
	Start() error
}

//WorkerManager singleton
type WorkerManager struct {
	workerQueue []Worker
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
//TODO: Implement a queue of builder to really manage something...
func (w *WorkerManager) Build(build *Build) error {

	var launchFunc func(build *Build, targetSys string)

	switch build.ProjectToBuild.Configuration.BuildType {
	case "Docker":
		launchFunc = w.launchDockerBuild
	default:
		build.State = Fail
		return errors.New("Not a valid build type")
	}

	if build.TargetSys == "all" {
		for sysToBuild := range build.ProjectToBuild.Configuration.BuildInstructions {
			launchFunc(build, sysToBuild)
		}
	} else {
		launchFunc(build, build.TargetSys)
	}
	return nil
}

func (w *WorkerManager) launchDockerBuild(build *Build, targetSys string) {
	d := DockerWorker{
		build:     *build,
		targetSys: targetSys,
	}
	d.Start()
}
