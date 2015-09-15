package controllers

import (
	"archive/tar"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
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
	StartDate            time.Time
	LastUpdated          time.Time
	ProjectToBuild       Project
	TargetSys            string
	State                State
	Commit               string
	UpdateWorkerDuration time.Duration
	Deploy               bool
	GitCommitID          string
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
	if b.LastUpdated.IsZero() || b.StartDate.IsZero() {
		return 0
	}
	if b.State == Building || b.State == Init {
		return time.Now().Round(time.Second).Sub(b.StartDate.Round(time.Second))
	}
	return b.LastUpdated.Round(time.Second).Sub(b.StartDate.Round(time.Second))
}

//CreateOutputTar the entire output folder
func (b *Build) CreateOutputTar() error {

	output := fmt.Sprintf("%s/public/output/%s/%d/%s/", revel.BasePath, b.ProjectToBuild.Name, b.Date.Unix(), b.TargetSys)
	tarFile, err := os.Create(output + "/output.tar")
	if err != nil {
		return err
	}
	tw := tar.NewWriter(tarFile)
	files, _ := ioutil.ReadDir(output)
	for _, f := range files {
		if f.Name() == "output.tar" || f.IsDir() == true {
			continue
		}
		fileFD, err := os.Open(output + "/" + f.Name())
		if err != nil {
			return err
		}
		defer fileFD.Close()
		stat, _ := fileFD.Stat()
		hdr := &tar.Header{
			Name: f.Name(),
			Mode: 0600,
			Size: stat.Size(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		content, _ := ioutil.ReadAll(fileFD)
		if _, err := tw.Write(content); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return err
	}
	return nil
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
			build = Build{ID: bson.NewObjectId(),
				Date:           time.Now(),
				ProjectToBuild: project,
				TargetSys:      sysToBuild,
				State:          Created,
				Commit:         commit,
				Deploy:         deploy,
				GitCommitID:    project.GetHeadCommitID(),
			}
			WMInstance().Build(&build)
			b.saveBuild(&build)
		}
	} else {
		build = Build{
			ID:             bson.NewObjectId(),
			Date:           time.Now(),
			ProjectToBuild: project,
			TargetSys:      sys,
			State:          Created,
			Commit:         commit,
			Deploy:         deploy,
			GitCommitID:    project.GetHeadCommitID(),
		}
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
	err := c.Update(bson.M{"_id": build.ID},
		bson.M{"$set": bson.M{"state": build.State, "lastupdated": time.Now(), "updateworkerduration": build.UpdateWorkerDuration, "startdate": build.StartDate}})
	if err != nil {
		log.Println(err)
	}
	if build.State > Fail && build.Deploy == true {
		b.Deploy(build)
	} else if build.State == Fail && build.Deploy == true {
		MMInstance().SendBuildFailedMail(*build)
	}
	return err
}

//Deploy the built package
func (b *BuildManager) Deploy(build *Build) {
	output := fmt.Sprintf("%s/public/output/%s/%d/%s/", revel.BasePath, build.ProjectToBuild.Name, build.Date.Unix(), build.TargetSys)

	localTmpFolder, _ := revel.Config.String("local_tmp_folder")
	tmpFolder := fmt.Sprintf("%s%s/packages/%s", localTmpFolder, build.ProjectToBuild.Name, strings.Replace(build.TargetSys, "_i386", "", -1))

	exec.Command("rm", "-Rf", tmpFolder).Run()
	exec.Command("mkdir", "-p", tmpFolder).Run()

	//Copy output to tmp_folder
	files, _ := ioutil.ReadDir(output)
	date := build.Date.Format("200601021504")
	re := regexp.MustCompile("(_amd64|_i386|\\.x86_64|\\.i686)?(\\.deb|\\.exe)$")
	for _, f := range files {
		exec.Command("cp", output+f.Name(), tmpFolder+"/"+re.ReplaceAllString(f.Name(), "-"+date+"~git"+build.GitCommitID+"$1$2")).Run()
	}

	//Exec Deploy Script
	cmd := exec.Command("/bin/bash", fmt.Sprintf("%s/scripts/%s", revel.BasePath, build.ProjectToBuild.Configuration.DeployScript), build.ProjectToBuild.Name)
	out, err := cmd.CombinedOutput()
	revel.WARN.Println(string(out))
	if err != nil {
		revel.ERROR.Println(err.Error())
	}
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
