package main

import (
    "io"
    "net/http"
    "os"
    "encoding/xml"
    "fmt"
    "io/ioutil"
    "os/exec"
    "strings"
    //"regexp"
)

type Directory struct {
  XMLName xml.Name `xml:"directory"`
  RepoListings []RepoListing `xml:"repo"`
}
type RepoListing struct {
  Name string `xml:"name"`
  Url string `xml:"url"`
  Fingerprint string `xml:"fingerprint"`
  Contents *Repo
}

type Repo struct {
  XMLName xml.Name `xml:"fdroid"`
  // Once golang fixes its crap move everything in the Metadata struct to this struct
  Info Metadata `xml:"repo"`
  Apps []App `xml:"application"`
}

func (self* Repo) DownloadPackage(pkg* Pkg) {
  os.MkdirAll("/var/lib/cskt/apks/", os.ModePerm)
  DownloadFile("/var/lib/cskt/apks/"+pkg.ApkName, self.Info.Url + "/" + pkg.ApkName)
}

type Metadata struct {
  XMLName xml.Name `xml:"repo"`
  Name string `xml:"name,attr"`
  Description string `xml:"description"`
  Url string `xml:"url,attr"`
  Pubkey string `xml:"pubkey,attr"`
  Version int `xml:"version,attr"`
  Timestamp int `xml:"timestamp,attr"`
}

type App struct {
  XMLName xml.Name `xml:"application"`
  Id string `xml:"id"`
  Name string `xml:"name"`
  Added string `xml:"added"`
  LastUpdated string `xml:"lastupdated"`
  License string `xml:"license"`
  Source string `xml:"source"`
  Tracker string `xml:"tracker"`
  Antifeatures string `xml:"antifeatures"`
  RecommendedVersion string `xml:"marketversion"`
  RecommendedVersionCode int `xml:"marketvercode"`
  /*Description string `xml:"desc"`*/
  Pkgs []Pkg `xml:"package"`
}

func (self* App) GetRecommendedPackage() *Pkg {
  for i := range self.Pkgs {
    if (self.Pkgs[i].VersionCode == self.RecommendedVersionCode) {
      return &self.Pkgs[i]
    }
  }
  return nil
}

type Pkg struct {
  XMLName xml.Name `xml:"package"`
  App* App
  Repo* RepoListing
  ApkName string `xml:"apkname"`
  SrcName string `xml:"srcname"`
  Version string `xml:"version"`
  VersionCode int `xml:"versioncode"`
  Size int `xml:"size"`
  MinSdk int `xml:"sdkver"`
  TargetSdk int `xml:"targetSdkVersion"`
  AddedDate string `xml:"added"`
  Permissions string `xml:"permissions"`
  //Just assume everything is SHA256 until golang fixes its xml lib
  Hash string `xml:"hash"`
  Sig string `xml:"sig"`
}

// DownloadFile will download a url to a local file. It's efficient because it will
// write as it downloads and not load the whole file into memory.
func DownloadFile(filepath string, url string) error {
    // Create the file
    out, err := os.Create(filepath)
    if err != nil {
        return err
    }
    defer out.Close()

    // Get the data
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    // Write the body to file
    _, err = io.Copy(out, resp.Body)
    if err != nil {
        return err
    }

    return nil
}

func (self* RepoListing) Download() {
  os.MkdirAll("/var/lib/cskt/repos/"+self.Name, os.ModePerm)

  err := DownloadFile("/var/lib/cskt/repos/"+self.Name+"/index.xml", self.Url+"/index.xml")
  if err != nil {
      panic(err)
  }
}

func LoadDirectory() *Directory {
  //Load directory information
  os.MkdirAll("/etc/cskt", os.ModePerm)
  xmlFile, err := os.OpenFile("/etc/cskt/directory.xml", os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println(err)
	}
  defer xmlFile.Close()

  byteValue, _ := ioutil.ReadAll(xmlFile)
  var directory Directory
  xml.Unmarshal(byteValue, &directory)

  //Load repo data
  for i := range directory.RepoListings {
  	xmlFile, err := os.Open("/var/lib/cskt/repos/"+directory.RepoListings[i].Name+"/index.xml")
  	if err != nil {
  		fmt.Println(err)
  	}
  	defer xmlFile.Close()

    byteValue, _ := ioutil.ReadAll(xmlFile)
    var repo Repo
    xml.Unmarshal(byteValue, &repo)
    directory.RepoListings[i].Contents = &repo

    for ii := range repo.Apps {
      for iii := range repo.Apps[ii].Pkgs {
        repo.Apps[ii].Pkgs[iii].App = &repo.Apps[ii]
        repo.Apps[ii].Pkgs[iii].Repo = &directory.RepoListings[i]
      }
    }

    fmt.Println("Found repository " + directory.RepoListings[i].Name)
  }

  return &directory
}

func (self* Directory) Write() {
  f, err := os.Create("/etc/cskt/directory.xml") // create/truncate the file
  if err != nil {
    fmt.Println(err)
  } // panic if error
  defer f.Close() // make sure it gets closed after

  encoder := xml.NewEncoder(f)
  err = encoder.Encode(self)
  if err != nil {
    fmt.Println(err)
  }
}

func (self* Directory) Update() {
  for i := range self.RepoListings {
    repo := self.RepoListings[i]
    fmt.Println("Downloading repository information for "+repo.Name)
    repo.Download()
  }
}

func CsktUpdate() {
  directory := LoadDirectory()
  directory.Update()
  directory.Write()
}

func (self* App) Find(version string) *[]*Pkg {
  candidates := make([]*Pkg,0)
  for i := range self.Pkgs {
    if self.Pkgs[i].Version == version {
      candidates = append(candidates, &self.Pkgs[i])
    }
  }
  return &candidates
}

func (self* Repo) Find(request PackageRequest) *[]*Pkg {
  candidates := make([]*Pkg,0)

  for i := range self.Apps {
    if strings.HasSuffix(self.Apps[i].Id, request.IdRequest) {
      if request.VersionName != "" {
        candidates = append(candidates, (*self.Apps[i].Find(request.VersionName))...)
      } else {
        recommendedPkg := self.Apps[i].GetRecommendedPackage()
        candidates = append(candidates, recommendedPkg)
      }
    }
  }
  return &candidates
}
func (self* Directory) Find(request PackageRequest) *[]*Pkg {
  candidates := make([]*Pkg,0)

  for i := range self.RepoListings {
    candidates = append(candidates, *(self.RepoListings[i].Contents.Find(request))...)
    for ii := range candidates {
      candidates[ii].Repo = &self.RepoListings[i]
    }
  }
  return &candidates
}

type PackageRequest struct {
  IdRequest string
  VersionName string
}

func ParsePackageRequest(request string) PackageRequest {
  parsed := strings.Split(request, ":")
  if len(parsed) > 1 {
    return PackageRequest{parsed[0],parsed[1]}
  } else {
    return PackageRequest{parsed[0],""}
  }
}

func DownloadApk(directory *Directory, request string) *Pkg {
  var candidates *[]*Pkg
  candidates = directory.Find(ParsePackageRequest(request))

  if len(*candidates) == 0 {
    fmt.Println("Package not found")
    return nil
  } else if len(*candidates) == 1 {
    pkg := (*candidates)[0]
    fmt.Println("Downloading "+pkg.App.Name+" "+pkg.Version)

    pkg.Repo.Contents.DownloadPackage(pkg)
    return pkg
  } else {
    fmt.Println("Multiple candidates:")
    for i := range *candidates {
      pkg := (*candidates)[i]
      fmt.Println(pkg.App.Id)
    }
    return nil
  }
}

func CsktDownload(directory *Directory, request string) {
  pkg := DownloadApk(directory, request)
  fmt.Println("Saved as "+pkg.ApkName)
}

type PackageList struct {
  Apps []InstalledPackage `xml:"package"`;
}
type InstalledPackage struct {
  Id string `xml:"id"`
  AppName string `xml:"name"`
  Index int `xml:"index"`
  Version string `xml:"version"`
  VersionCode int `xml:"versioncode"`
}

func CsktInstall(directory *Directory, request string) {
  //We need to make sure we don't create duplicates
  //Prepare the package list (should we put this in a function :thinking:)
  xmlFile, err := os.OpenFile("/var/lib/cskt/packages.xml", os.O_RDWR|os.O_CREATE, 0666)
  if err != nil {
    fmt.Println(err)
  }
  byteValue, _ := ioutil.ReadAll(xmlFile)
  var packageList PackageList
  xml.Unmarshal(byteValue, &packageList)
  installed := packageList.IsInstalled(ParsePackageRequest(request))
  if installed != nil {
    fmt.Println(installed.AppName + " is already installed with version " + installed.Version)
  } else {
    pkg := DownloadApk(directory, request)
    cmd := exec.Command("pm","install","/var/lib/cskt/apks/"+pkg.ApkName)
    err = cmd.Run()
    if err != nil {
      fmt.Println(err)
    } else {
      packageList.Apps = append(packageList.Apps, InstalledPackage{pkg.App.Id, pkg.App.Name, len(packageList.Apps), pkg.Version,pkg.VersionCode})
      byteValue, _ = xml.Marshal(packageList)
      xmlFile.Truncate(0)
      xmlFile.Seek(0,0)
      xmlFile.Write(byteValue)
      fmt.Println("Successfully installed "+pkg.App.Name+" "+pkg.Version)
    }
    os.Remove("/var/lib/cskt/apks/"+pkg.ApkName)
  }
}

func CsktUninstall(directory *Directory, id string) {
  candidates := make([]*InstalledPackage,0)
  //Add to the package list
  xmlFile, err := os.OpenFile("/var/lib/cskt/packages.xml", os.O_RDWR|os.O_CREATE, 0666)
  if err != nil {
    fmt.Println(err)
  }
  byteValue, _ := ioutil.ReadAll(xmlFile)
  var packageList PackageList
  xml.Unmarshal(byteValue, &packageList)

  //Find all possiblities
  for i := range packageList.Apps {
    if strings.HasSuffix(packageList.Apps[i].Id,id) {
      candidates = append(candidates, &packageList.Apps[i])
    }
  }
  if len(candidates) == 0 {
    fmt.Println("No such package")
  } else if len(candidates) == 1 {
    cmd := exec.Command("pm","uninstall",candidates[0].Id)
    err := cmd.Run()
    if err != nil {
      fmt.Println(err)
    } else {
      packageList.Apps = append(packageList.Apps[:candidates[0].Index], packageList.Apps[candidates[0].Index+1:]...)
      byteValue, _ = xml.Marshal(packageList)
      xmlFile.Truncate(0)
      xmlFile.Seek(0,0)
      xmlFile.Write(byteValue)
      //Make this the actual name to be more user friendly
      fmt.Println("Uninstalled "+candidates[0].Id)
    }
  } else {
    fmt.Println("Multiple candidates:")
    for i := range candidates {
      pkg := candidates[i]
      fmt.Println(pkg.Id)
    }
  }
}

func (self* Directory) Add(url string, fingerprint string) {
  //This is going to screw up if we run more than one instance of casket
  os.MkdirAll("/tmp/cskt", os.ModePerm)
  DownloadFile("/tmp/cskt/repo_info.xml", url+"/index.xml")
  xmlFile, err := os.OpenFile("/tmp/cskt/repo_info.xml", os.O_RDONLY|os.O_CREATE, 0666)

  // if os.Open returns an error then handle it
  if err != nil {
    fmt.Println(err)
  }
  // defer the closing of our xmlFile so that we can parse it later on
  defer xmlFile.Close()
  byteValue, _ := ioutil.ReadAll(xmlFile)
  var repo Repo
  xml.Unmarshal(byteValue, &repo)
  for i := range self.RepoListings {
    if (self.RepoListings[i].Url == url) {
      fmt.Println("Repository "+self.RepoListings[i].Name+" already exists")
      return
    }
  }

  newListing := RepoListing{Name: repo.Info.Name, Url: url, Fingerprint: fingerprint}
  self.RepoListings = append(self.RepoListings, newListing)
  fmt.Println("Added repository "+repo.Info.Name)
  byteValue, _ = xml.Marshal(self)

  ioutil.WriteFile("/etc/cskt/directory.xml",byteValue,0644)
  self.Update()
}

func (self* Directory) Remove(url string) {
  name := ""
  found := false

  for i := range self.RepoListings {
    if (self.RepoListings[i].Url == url) {
      name = self.RepoListings[i].Name
      self.RepoListings = append(self.RepoListings[:i], self.RepoListings[i+1:]...)
      found = true;
    }
  }

  if found {
    byteValue, _ := xml.Marshal(self)
    ioutil.WriteFile("/etc/cskt/directory.xml",byteValue,0644)
    fmt.Println("Removed repository "+name)
  } else {
    fmt.Println("No such repository found, make sure you entered the correct URL")
  }
}

func (self* Directory) List() {
  for i := range self.RepoListings {
    fmt.Println(self.RepoListings[i].Url)
  }
}

//Looks in the package list to see if a package is already installed
func (self* PackageList) IsInstalled(request PackageRequest) *InstalledPackage {
  for i := range self.Apps {
    if strings.HasSuffix(self.Apps[i].Id, request.IdRequest) {
      return &self.Apps[i]
    }
  }
  return nil
}

func main() {
  switch os.Args[1] {
    case "repo":
      switch os.Args[2] {
        case "add":
          directory := LoadDirectory()
          directory.Add(os.Args[3],"")
        case "remove":
          directory := LoadDirectory()
          directory.Remove(os.Args[3])
        case "list":
          directory := LoadDirectory()
          directory.List()
      }

    case "update":
      CsktUpdate()
    case "download":
      directory := LoadDirectory()
      CsktDownload(directory, os.Args[2])

    case "install":
      directory := LoadDirectory()
      CsktInstall(directory, os.Args[2])
    case "uninstall":
      directory := LoadDirectory()
      CsktUninstall(directory, os.Args[2])

    case "g":
      fmt.Println("WHEN I WAS\nA YOUNG BOY\nMY FATHER\nTOOK ME INTO THE CITY\nTO SEE A MARCHING BAND")
  }
}
