---
title: "Kubernetes Control Plane natively on FreeBSD with K3s" 
date: 2020-12-30T17:55:19-03:00 # Date of post creation.
description: "How to run Kubernetes Control Plane on FreeBSD just for fun (or for pain!)" 
featured: true # Sets if post is a featured post, making appear on the home page side bar.
draft: false # Sets whether to render this page. Draft of true will not be rendered.
toc: true # Controls if a table of contents should be generated for first-level links automatically.
codeMaxLines: 100 # Override global value for how many lines within a code block before auto-collapsing.
codeLineNumbers: true # Override global value for showing of line numbers within code block.
figurePositionShow: true # Override global value for showing the figure label.
categories:
  - Technology
tags:
  - FreeBSD
  - kubernetes
  - k3s
---

## Introduction

I've been trying, on the past few days make Kubernetes usable on FreeBSD. Why? Some would say I like to suffer. Others would say I have much spare time.

To be honest, I'm doing this for fun. [FreeBSD](https://www.freebsd.org), for me, is one of the most stable and performant Operating Systems, and it's sad that we cannot
use it to run Kubernetes components (yet).

In a near future, me and [Karen Bruner](https://twitter.com/fuzzykb) are planning to make not only the control plane usable, but also to use one of the precedents of containers on
modern operating systems, even before Docker: [Jails](https://www.freebsd.org/doc/handbook/jails.html) as a "container runtime".

I'm also using [k3s](https://k3s.io/) because I became too lazy to configure Kubernetes the hard way :P So bootstrapping an etcd, then the components was too much (for now) 
just to have some fun. But maybe some day, will do.

**Spoiler alert**: All the main components of Kubernetes can be compiled for FreeBSD. An exception, right now is just [kubelet](https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/)
that have, in a method a wrong syscall to fetch the node boottime, but a [PR](https://github.com/kubernetes/kubernetes/pull/97270) was created to correct that.

## Removing code before compiling

So K3s is a AIO (all in one) binary that runs all the components necessary to make Kubernetes works. You can use Alex Ellis [k3sup](https://github.com/alexellis/k3sup)
to bootstrap a Kubernetes cluster really, really fast.

Karen has also shown [how to do this with FreeBSD](https://productionwithscissors.run/2020/12/26/adventures-in-freebernetes-tutorial-build-your-own-bare-vm-k3s-cluster/) as a virtualization host + Linux machines running over bhyve (It's a FreeBSD + bhyve + Linux + k3sup) and you should really check this!

But going back to the main subject: k3s does not natively compile for FreeBSD. And why this happens? Because it needs some components that can only be compiled on Linux (or Windows) to work.

As an example, it embeds [kube-proxy](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-proxy/). But, kube-proxy needs [iptables](https://en.wikipedia.org/wiki/Iptabl) and ipvs(https://en.wikipedia.org/wiki/IP_Virtual_Server) to compile, which are Linux native technologies, not present on FreeBSD kernel.

The same happens with [flannel](https://github.com/coreos/flannel), which is embedded in the code and used as the [CNI](https://github.com/containernetworking/cni).

Fortunately, all of this code are present on a main package of k3s: **agent**. But, we don't need agent for our Control Plane, because agent is responsible for behaving like...AN AGENT for k3s :D

So, moving on, you can see the necessary modifications to make k3s compile for FreeBSD here: https://github.com/k3s-io/k3s/compare/v1.20.0+k3s2...rikatz:fbsd-control-plane

Explaining:

* There are some files I hadn't changed, but appear as difference because I've made all the mods from upstream branch
* On the `main.go` files (cmd/k3s/main.go and main.go) the call to "agent" packages is removed. The resulting binary wont have "agent" subcommand, but only "server", "kubectl", etc.
* On `pkg/cli/server/server.go` all the references for `agent` and `rootless` are removed. This way, we are not using anything from `agent` package that can stop us from compiling!
* On `pkg/daemons/executor` interface, all the references for Kubelet and KubeProxy are removed. We're running control plane, and as both of those components does not compile in FreeBSD, let's get rid of them
* On `pkg/server/server.go` all the references for rootless/rootlessports are removed, because this package uses some native Linux calls to make rootless works (and we're not running anything as a container anyway!)

There's one more item to take note: On [server.go](https://github.com/rikatz/k3s/blob/fbsd-control-plane/pkg/cli/server/server.go#L47) k3s uses an artifact to masquerade its process to not expose some credential passed as an argument.

But the library used (gspt) uses some headers and some sort of CGO, so you can make two choices here:

* Remove the mentioned line (and the referenced library) and be aware someone using ps wwwaux can see your credentials
* Keep the line, but compile on a FreeBSD machine (instead of using GOOS=freebsd on a Linux machine). Also, be aware that **WITH THIS OPTION K3S DOES NOT COMPILE FOR RPI3/ARM64** (or maybe you need a rpi3 and compile there, which is plausible but I didn't tried!)

One alternative here (and I need to check if this is possible) is to replace this to some syscall.

## Compiling
So now that all the modifications have been explained, compiling is pretty easy:

* If you are on a Linux machine (and removed the gspt line mentioned above)
```
GOOS=freebsd go build -o k3s main.go
```

You can put also a GOARCH=arm64 together with GOOS=freebsd and compile it for RPi3

* If you are on a FreeBSD machine:
```
go build -o k3s main.go
```

Aaaand that's it!

## Running
Well, so here we got a problem. K3s was built to run as a standalone AIO with embedded database (sqlite), or you can use an external database (postgres, mysql, etcd, etc).

But, it happens that sqlite is built with CGO, and because we don't have it on FreeBSD the best option FOR ME was: install a default Postgres and make it run.

Here a side note: I didn't put much effort on trying to make sqlite works, and instead I've directly jumped to Postgres because it was the easiest path. But, be my guest, and tell me what have you discovered!

Well, so, to make it run we need:
* A Postgres DB
* The generated binary

Installing Postgres on FreeBSD is really, really simple but yes, you can run this on an external DB running on Linux, in some cloud, etc.

But let's install this DB. Assuming you have a FreeBSD box working:

```
pkg update
pkg install postgresql12-server postgresql12-client 
sysrc postgresql_enable=yes
/usr/local/etc/rc.d/postgresql initdb
service postgresql start
```

And that's it for PostgreSQL. Obviously, this is a VERY INSECURE lab setup! Don't run this (or anything in this tutorial, to be fair) in Production!

Well, with Postgres up and running, you can run k3s and your control plane will be magically bootstrapped:

```
./k3s server --datastore-endpoint=postgres://postgres@127.0.0.1?sslmode=disable
```

This is going to print a lot of messages in your terminal, and now you have a Kubernetes Control Plane up and running natively on FreeBSD. Nice, right?

## Testing
Well, ok, the daemon is running but PROVE ME IT WORKS!

Okie, so first of all, you need somewhere to run a kubectl. You can run it on FreeBSD (it's easy to compile! I promise!) but I wont go that way right now.

So, grab the kubeconfig you're going to use on `/var/lib/rancher/k3s/server/cred/admin.kubeconfig`, plus the referenced certificates in `/var/lib/rancher/k3s/server/tls/client-*` and `/var/lib/rancher/k3s/server/tls/server-ca.crt`, and copy them to another machine that kubectl is installed.

Edit the admin.kubeconfig file, changing:
* `server: https://127.0.0.1:6444` -> `server: https://fbsd-ip:6443`
* `certificate-authority`, `client-certificate` and `client-key` to point to certificates location. If they're on the same directory as admin.kubeconfig, just remove the leading path and leave only the file name

Great! So now let's check if kubectl can communicate with the server and what it brings to us:

```
kubectl --kubeconfig=admin.kubeconfig version
Client Version: version.Info{Major:"1", Minor:"20", GitVersion:"v1.20.1", GitCommit:"c4d752765b3bbac2237bf87cf0b1c2e307844666", GitTreeState:"clean", BuildDate:"2020-12-18T12:09:25Z", GoVersion:"go1.15.5", Compiler:"gc", Platform:"linux/amd64"}
Server Version: version.Info{Major:"1", Minor:"20", GitVersion:"v1.20.0-k3s1", GitCommit:"3559625e27197f60bcc39c4b3ecffc80bad7785e", GitTreeState:"clean", BuildDate:"2020-12-08T20:42:55Z", GoVersion:"go1.15.6", Compiler:"gc", Platform:"freebsd/amd64"}
```

So if you take a look above, you'll see that the Server Platform is: `"freebsd/amd64"` (I'm running as a VM here in my machine!)

"Nah, but this is not enough! I wanna see some Kubernetes Nodes!" - Me, talking with myself!

OK, so let's go. First, you need to install k3s binary on some Linux machine. Go ahead, and grab some [release](https://github.com/k3s-io/k3s#manual-download) 

Nice, so now we need the token to join our agent/node with our Control Plane. It's located on the Control Plane machine, so grab the token with:

```
cat /var/lib/rancher/k3s/server/token
```

With that token copied, go back to your Linux machine that you're going to run your node/agent and run:

```
sudo k3s agent --server https://fbsd-ip:6443 --token THE-TOKEN-YOU-TOOK-NOTE
```

After some time, your node will appear for the Control Plane. You can check this way:

```
kubectl --kubeconfig=admin.kubeconfig get nodes
NAME      STATUS   ROLES    AGE   VERSION
node123   Ready    <none>   68m   v1.20.0+k3s2
```

Go ahead, run some Pods, be happy :D