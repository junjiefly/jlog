# jlog
a logging tool similar to glog style, inherits log compression and rollback functionalities

eg:
func main(){
  for i:=0;i<1000;i++{
    jlog.V(0).Infoln("log index:",i)
  }
  jlog.Shutdown()
}

go build
./xxx jlog -logDir=./eg
  
