I do not have a linux box for development so I set up a Docker container using
Docker for Windows.  Included in the repo are the required configuration files
for the Docker container to start.

After successfully starting the host_monitor application below, to access the report
from a browser on your PC go to http://localhost:8080

The make file will run in Linux, just skip to the <Linux> tag below to run.

Prerequsites:
	Docker for Windows installed

Instructions to run from windows:
	from powershell (in the directory with the files):
		> docker run --rm -it -v "$($pwd):/app" -w /app -p 8080:8080 golang:1.21-alpine /bin/sh
			starts the docker container sharing resources for the applicatino and connect to the linux terminal
		
		# apk add make    
			installs make into the linux environment
	
	You will now have a functional linux environment to run the program from windows.

<Linux>
	Using the Makefile in this directory, the following build and run the program:
		# make build
			builds the application
			
		# make run
			runs the program with defaults
		
		alternatively:
		# ./host-monitor --hosts=actiontarget.com,ksl.com --interval=5000
			use , seperated list for hosts; --interval will be set to 2000 if not included
	
To run docker, install make, compile code, run default:
	> docker run --rm -it -v "$($pwd):/app" -w /app -p 8080:8080 golang:1.21-alpine /bin/sh -c "apk add --no-cache make && make run"
	
	
From Powershell not using linux terminal:
	Rename the Makefile_Docker to Makefile
	
	> docker build -t host_monitor .
		this will build the host_monitor 
	
	> docker run --rm -it -p 8080:8080 --name host_monitor-custom host_monitor -hosts google.com,microsoft.com,martindoor.com -interval 5000
		starts the docker container, shares the port, then adds hosts and interval 
