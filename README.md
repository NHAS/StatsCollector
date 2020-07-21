# StatsCollector

An extremely basic group server monitoring solution. Use at your own risk.  
Decided that I wanted to try my hand at creating something to send me emails and let me view basic stats about the computers I monitor.  

An agent based approach is utilised here, `Iris` as the client and `Theia` as the server/collector.  

This project also heavily heavily uses the golang SSH library to manage authentication of clients and servers. 




## Local Machine Install

Install `postgres`, through whatever you distro allows (e.g pacman, yum, apt so on).  

Current the DB connection can only be localhost, without SSL with the user `gorm` and the database of `stats`. These are not hard limitations and could be easily changed by editing `cmd/theia/main.go`.  

As such create a user `gorm`


```
sudo -u postgres psql
postgres=# create database stats;
postgres=# create user gorm with encrypted password '<ENTER A GOOD PASSWORD HERE>';
postgres=# grant all privileges on database gorm to stats;

```
Adapted from <a href="https://medium.com/coding-blocks/creating-user-database-and-adding-access-on-postgresql-8bfcd2f4a91e">here</a>

Get the project and build it.
```
go get github.com/NHAS/StatsCollector
cd ~/go/src/github.com/NHAS/StatsCollector/
mkdir bin/
cd bin/
go build -o ../...
```


Create the file structure to seperate the client/server config and key information.

```
mkdir {client,server}
```

Generate keypairs for both the client and server with ssh-keygen.

```
ssh-keygen -t ed25519 -f server/id_ed25519
ssh-keygen -t ed25519 -f client/id_ed25519
```


Sample server config into `server/`:

```
{
	"ssh_listen_addr": ":2222",
	"web_interface_addr": ":8080",
	"private_key_path": "./server/id_ed25519",
	"web_path": "/home/<YOUR USERNAME>/go/src/github.com/NHAS/StatsCollector/resources"
}
```

Sample client config into `client/`:

```
{
	"server_address": "127.0.0.1:2222",
	"authorised_key": "<SERVER AUTHORISED KEY>",
	"monitor_urls": [
		{
			"url": "https://endpointyouwanttocheck.com",
			"ok_code": 200,
			"timeout": 5
		}
	],
	"private_key_path": "./client/id_ed25519"
}
```


Add a user with `theia` (this will prompt for username & pwd):
```
./theia -adduser
```

Then start server with (this assumes postgres is running):

```
PASSWORD="<ENTER A GOOD PASSWORD HERE>" ./theia -config server/config.json -log server/log.txt 
```

And then client: 

```
./iris -config client/config.json -log client/log.   
```

Open `localhost:8080` in a browser and login with previously created creds. 
Finally add the clients public key under `Add Agent` section in the top right. 


## Deployment

Unfortunately I havent gotten around to making anything more automated. But below you'll find the `systemd` service files to run these as services.  
From them you should be able to infer what paths you could put things. 

But its up to you. 

```
[Unit]
Description=Theia Stats Collector Server
After=network.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
Restart=always
RestartSec=1
User=theia
ExecStart=/usr/local/bin/theia -config /usr/local/etc/theia/config.json -log /var/log/theia/log.txt
Environment=PASSWORD=<YOUR MASSIVELY GOOD PASSWORD>

[Install]
WantedBy=multi-user.target
```

```
[Unit]
Description=Iris Stats Collector Client
After=network.target

[Service]
Type=simple
Restart=always
RestartSec=1
User=iris
ExecStart=/usr/local/bin/iris -config /usr/local/etc/iris/config.json -log /var/log/iris/log.txt

[Install]
WantedBy=multi-user.target
```

## Current Features

- Email alerts on selected hosts about disk usage, endpoint failure or host failure
- SSH pub key based auth
- Web ready authentication
- Basic metric collection of memory, disk and network services
- Basic user management 

## Limitations

- Features no history for metrics (memory/disk)
- All users are administrators
- Email host configuration (the thing that sends the email) is a bit jank at the moment
- Events arent displayed with very useful information as of yet
- Dashboard is quite information sparse
- Renaming of agents isnt possible through the web interface as of yet
- If monitor of an endpoint is removed client side, it is not updated server side

## Todo

- Rename agents in web interface
- Rework email notifications to be user specific
- Rework disk utilisation alerts to be disk specific, so you can disable alerts on loopback devices
- Add more useful information to the dashboard when all hosts are up
- Create automated deployement script, or look into packaging 