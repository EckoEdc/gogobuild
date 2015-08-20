# GoGo Build

GoGo Build is a build/packaging system designed to be light and resilient.

# Setup
 
 Dependencies
 * go get github.com/fsouza/go-dockerclient
 * go get gopkg.in/mgo.v2
 * go get golang.org/x/build/gerrit
 
Project
 * go get github.com/EckoEdc/gogobuild

# Project Setup
 Just clone your project in public/project and make a .packer.json describing
 how GoGo Build should build it. (you'll find an example of this file in project/example-project)

# Run it
 revel run github.com/EckoEdc/gogobuild

 go to http://localhost:9000 with your favorite browser

 *Note: There is still a lot of work to do on this, consider it early alpha.
 All comments and pull request are welcome*
