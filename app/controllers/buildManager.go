package controllers

import (
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/revel/revel"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

//State enum
type State int

//State enum *Keep this ordered*
const (
	Created  State = iota //0
	Init                  //1
	Building              //2
	Fail                  //3

	Success         //4
	FallbackSuccess //5
)

func (s State) String() string {
	switch s {
	case Created:
		return "Created"
	case Init:
		return "Init"
	case Building:
		return "Building"
	case Fail:
		return "Fail"
	case Success:
		return "Success"
	case FallbackSuccess:
		return "FallbackSuccess"
	}
	return "Unknown"
}

//Build represent a build
type Build struct {
	ID                   bson.ObjectId `bson:"_id,omitempty"`
	Date                 time.Time
	LastUpdated          time.Time
	ProjectToBuild       Project
	TargetSys            string
	State                State
	Commit               string
	UpdateWorkerDuration time.Duration
	Deploy               bool
}

//IsDownloadable return true if downloadable
func (b *Build) IsDownloadable() bool {
	if b.State > Fail && b.Commit != "updateWorker" {
		return true
	}
	return false
}

//IsRetryable return true if we can retry a failed build
func (b *Build) IsRetryable() bool {
	if b.Commit == "master" {
		return false
	}
	if b.State == Fail {
		return true
	}
	return false
}

//IsDeployable return if the build can be deployed
func (b *Build) IsDeployable() bool {
	if b.State > Fail && b.Commit != "updateWorker" && b.Commit == "master" {
		return true
	}
	return false
}

//Duration return diff between date and updatedDate
func (b *Build) Duration() time.Duration {
	if b.LastUpdated.IsZero() {
		return 0
	}
	if b.State == Building || b.State == Init {
		return time.Now().Round(time.Second).Sub(b.Date.Round(time.Second))
	}
	return b.LastUpdated.Round(time.Second).Sub(b.Date.Round(time.Second))
}

//BuildManager is the build manager
type BuildManager struct {
	session *mgo.Session
}

//instance of BuildManager
var bmInstance *BuildManager

//BMInstance Return the instance of build manager
func BMInstance() *BuildManager {
	if bmInstance == nil {
		bmInstance = new(BuildManager)
		var err error
		bmInstance.session, err = mgo.Dial("127.0.0.1")
		if err != nil {
			//Pretty much dead if we don't have mongoDB anyway
			log.Fatal(err)
		}
	}
	return bmInstance
}

//CreateOrReturnStatusBuild create or return status of requested build
func (b *BuildManager) CreateOrReturnStatusBuild(projectName string, sys string, commit string, deploy bool) (*Build, error) {
	//FIXME: This logic is flawed
	// if commit == "master" || commit == "updateWorker" {
	// 	return b.newBuild(projectName, sys, commit), nil
	// }
	// c := b.session.DB("gogobuild").C("builds")
	// var build = new(Build)
	// err := c.Find(bson.M{"projecttobuild.name": projectName, "targetsys": sys, "commit": commit}).One(build)
	// if err != nil && err.Error() == "not found" {
	// 	b.newBuild(projectName, sys, commit)
	// } else if build.State == Fail {
	// 	b.RetryBuild(build)
	// }
	return b.newBuild(projectName, sys, commit, deploy), nil
}

//NewBuild create a build and gives it to WorkerManager
func (b *BuildManager) newBuild(projectName string, sys string, commit string, deploy bool) *Build {
	project := PMInstance().GetProjectByName(projectName)
	project.Reload()
	var build Build
	if sys == "all" {
		for sysToBuild := range project.Configuration.BuildInstructions {
			build = Build{ID: bson.NewObjectId(), Date: time.Now(), ProjectToBuild: project, TargetSys: sysToBuild, State: Created, Commit: commit, Deploy: deploy}
			WMInstance().Build(&build)
			b.saveBuild(&build)
		}
	} else {
		build = Build{ID: bson.NewObjectId(), Date: time.Now(), ProjectToBuild: project, TargetSys: sys, State: Created, Commit: commit, Deploy: deploy}
		WMInstance().Build(&build)
		b.saveBuild(&build)
	}
	return &build
}

//RetryBuild that failed
func (b *BuildManager) RetryBuild(build *Build) {
	build.ProjectToBuild.Reload()
	WMInstance().Build(build)
}

//GetBuildsByProjects get list of projects builds
func (b *BuildManager) GetBuildsByProjects(projectName string) ([]Build, error) {
	c := b.session.DB("gogobuild").C("builds")
	var buildList []Build
	err := c.Find(bson.M{"projecttobuild.name": projectName}).Sort("-date").All(&buildList)
	if err != nil {
		log.Println(err)
	}
	return buildList, err
}

//GetBuildByID return a build by it's id
func (b *BuildManager) GetBuildByID(id string) (*Build, error) {
	c := b.session.DB("gogobuild").C("builds")
	var build = new(Build)
	err := c.FindId(bson.ObjectIdHex(id)).One(build)
	if err != nil {
		log.Println(err)
	}
	return build, err
}

//UpdateBuild in DB
func (b *BuildManager) UpdateBuild(build *Build) error {
	c := b.session.DB("gogobuild").C("builds")
	err := c.Update(bson.M{"projecttobuild.name": build.ProjectToBuild.Name, "date": build.Date},
		bson.M{"$set": bson.M{"state": build.State, "lastupdated": time.Now(), "updateworkerduration": build.UpdateWorkerDuration}})
	if err != nil {
		log.Println(err)
	}
	if build.State > Fail && build.Deploy == true {
		b.Deploy(build)
	}
	return err
}

//Deploy the built package
func (b *BuildManager) Deploy(build *Build) {
	output := fmt.Sprintf("%s/public/output/%s/%d/%s/%s", revel.BasePath, build.ProjectToBuild.Name, build.Date.Unix(), build.TargetSys, build.ProjectToBuild.Configuration.Package[build.TargetSys])

	localRepoFolder, _ := revel.Config.String("local_repo_folder")
	packageDateName := fmt.Sprintf("%s/%s-%s", localRepoFolder, build.Date.Format("200601021504"), build.ProjectToBuild.Configuration.Package[build.TargetSys])

	//cp the package with date stamp
	exec.Command("cp", output, packageDateName).Run()

	//rm the old symbolic link and re-create it on the new build
	linkName := fmt.Sprintf("%s/%s", localRepoFolder, build.ProjectToBuild.Configuration.Package[build.TargetSys])
	exec.Command("rm", "-f", linkName).Run()
	exec.Command("cp", "-s", linkName, packageDateName).Run()

	distantUser, _ := revel.Config.String("distant_user")
	distantIP, _ := revel.Config.String("distant_ip")
	distantFolder, _ := revel.Config.String("distant_folder")

	//rsync the local repository to the distrubution server
	exec.Command("rsync", "-arv", localRepoFolder, fmt.Sprintf("%s@%s:%s", distantUser, distantIP, distantFolder)).Run()
}

//SaveBuild in DB
func (b *BuildManager) saveBuild(build *Build) error {
	c := b.session.DB("gogobuild").C("builds")
	err := c.Insert(build)
	if err != nil {
		log.Println(err)
	}
	return err
}

//BuildMaintenance should be called in case build were not updated to there final state
//(e.g at start since no build should be in created, init or building state)
func (b *BuildManager) BuildMaintenance() error {
	c := b.session.DB("gogobuild").C("builds")
	_, err := c.UpdateAll(bson.M{"state": bson.M{"$in": []State{Created, Init, Building}}},
		bson.M{"$set": bson.M{"state": Fail}})
	return err
}
