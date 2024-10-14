# Go AST Client 📜
Welcome to the **Go AudioSocket Client** project! This project is designed to help you work with AudioSocket connections and WebRTC VAD in Go.

## Features ✨

- Handle AudioSocket connections
- Process audio data with WebRTC VAD
- Transcribe audio and interact with a server
- WebSocket communication for real-time data exchange

## Installation 🛠️

To install and run this project, you need the following servers up and running:

- **Whisper Server**
- **xTTSv2 Server**
- **Ollama Server**

Make sure these servers are properly configured and accessible before proceeding with the installation.

### Install Dependencies

To install the necessary dependencies, run the following command:

```sh
go get
```

### Asterisk Server Configuration

You also need to run an Asterisk server with the following settings. There are two Asterisk implementations: a channel interface and a dialplan application interface. Each of these lends itself to simplify a different use-case, but they work in exactly the same way.

The following examples demonstrate an AudioSocket connection to a server at `server.example.com` running on TCP port `9092`. The UUID (which is chosen arbitrarily) of the call is `40325ec2-5efd-4bd3-805f-53576e581d13`.

#### Dialplan Application

```asterisk
exten = 100,1,Verbose("Call to AudioSocket via Dialplan Application")
 same = n,Answer()
 same = n,AudioSocket(40325ec2-5efd-4bd3-805f-53576e581d13,server.example.com:9092)
 same = n,Hangup()
```

#### Channel Interface

```asterisk
exten = 101,1,Verbose("Call to AudioSocket via Channel interface")
 same = n,Answer()
 same = n,Dial(AudioSocket/server.example.com:9092/40325ec2-5efd-4bd3-805f-53576e581d13)
 same = n,Hangup()
```

## Usage 🚀

To make a call from Asterisk, you can use the following command:

```sh
channel originate PJSIP/7000 extension 7000@from-internal
```
with settings
```asterisk
[from-internal]
exten = 7000,1,Verbose("Call to AudioSocket via Dialplan Application")
 same = n,Answer()
 same = n,AudioSocket(40325ec2-5efd-4bd3-805f-53576e581d13,localhost:9092)
 same = n,Hangup()
```
```toml
[7000]
type=endpoint
context=from-external
disallow=all
allow=ulaw
transport=transport-udp
auth=7000
aors=7000

[7000]
type=auth
auth_type=userpass
password=pass
username=7000

[7000]
type=aor
remove_existing=yes
max_contacts=1
```
This command will originate a call to the PJSIP endpoint `7000` and connect it to the extension `7000` in the `from-internal` context.

## TODO:

1. Review the [Whisper CPP Server](https://github.com/litongjava/whisper-cpp-server) and update the client implementation if necessary.
2. Implement a microphone port for local machine usage.
3. Improve transcription accuracy.

