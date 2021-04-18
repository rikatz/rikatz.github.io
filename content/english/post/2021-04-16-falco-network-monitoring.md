---
title: "Using Falco to monitor outbound traffic for Pods in Kubernetes"
date: 2021-04-16T16:32:27-03:00
draft: false
description: "How to use Falco to monitor the network traffic of your Pods"
featured: true # Sets if post is a featured post, making appear on the home page side bar.
toc: true 
codeMaxLines: 100 # Override global value for how many lines within a code block before auto-collapsing.
codeLineNumbers: true # Override global value for showing of line numbers within code block.
figurePositionShow: true # Override global value for showing the figure label.
categories:
  - Technology
tags:
  - Kubernetes
  - Falco
  - Security
  - Observability
---

# Using Falco to monitor outbound traffic for Pods in Kubernetes



[Falco](https://www.falco.org) is an opensource project from [Sysdig](https://sysdig.com/) focused on container runtime and cloud native security, that uses modern technologies like  [eBPF](https://ebpf.io) to monitor environment situations using [syscalls](https://man7.org/linux/man-pages/man2/syscalls.2.html) and other [events sources](https://falco.org/docs/event-sources/)

We've used Falco some time ago as a PoC to monitor some specific events, and recently I've started to mess with it a bit to help contributing on it as a request from the amazing [Dan Pop](https://twitter.com/danpopnyc/) and also to understand better how Falco could help me in future situations.

## The problem I needed to solve

Who uses Kubernetes knows how hard is to keep track of networking connections originated from Pods. Even using [Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/), it's hard to assess which Pod tried to open a connection to internal or external world.

Having this kind of information is essential in environments with a higher restriction level, or even if you need to legally answer "who was connecting to this network endpoint".

Some CNI providers have specific solutions, like [Antrea](https://antrea.io) exporting via [Netflow](https://antrea.io/docs/v1.0.0/network-flow-visibility/), or [Cilium](https://cilium.io/) with a short retention solution and visualization with project [Hubble](https://github.com/cilium/hubble).

But I wanted something more: I wanted a solution that could be applied with any CNI, even if it does not support Network Policy. I don't care if the connection was dropped or not, I wanted to know how to monitor every connection attempt my Pods does. And this can be monitored, as every connection generates a syscall.


## The final result

![Final result](/images/falcomonitoring/grafana.png)

## Let me introduce you Falco!

As I've explained before, Falco is an opensource project that monitors events in servers. Those events can be syscalls from containers, or even Kubernetes audit events.

Falco is based in [rules](https://falco.org/docs/rules/). Those rules mainly define:
* a common **name** (`rule`) - "Unauthorized SSH access"
* a **description** (`desc`) - "Some user attempted to login in an SSH service"
* a **priority** (`priority`) - "WARNING"
* a **detailed output** from the event (`output`) - "Server 'homer' received a connectin in port 22 from the non authorized IP 192.168.0.123"
* a **condition** (`cond`) - "(Server has the word 'restricted' in the hostname, and receives a connection in port 22 from a source that is not network 10.10.10.0/24) OR (server received a connection in port 22 mas the process that received this connection is not called 'sshd')
  
Here, I need to point something: I wrote the condition field above in a really simple form, as a common language. Besides Falco rules not being written exactly in this form (as we will see below), it's a really similar format, making it easier to read the rules.

The rules can also contain other fields not explained here, like exceptions, aditional tags, and also **lists** and **macros** that turns repetition of strings easier.

## Installing Falco

Before we begin to install Falco, a brief description of my environment:

* 3 virtual servers with [Flatcar Linux](https://kinvolk.io/flatcar-container-linux/) 2765.2.2, because I LOVE FLATCAR MODEL!! (and also it has an up to date Kernel, which allows me to use Falco eBPF driver). If you want to learn how to install Flatcar in 5 minutes using VMware Player, there's a blog post [here](/post/2020-09-13-flatcar/) explaining how.
* My physical networking is `192.168.0.0/24`
* My Kubernetes install is v1.21.0 via Kubeadm. Pods are creted in network `172.16.0.0/16`

To install Falco on Kubernetes using [helm](http://helm.sh), basically you need to follow 4 steps:
```bash
kubectl create ns falco
helm repo add falcosecurity https://falcosecurity.github.io/charts
helm repo update
helm install falco falcosecurity/falco --namespace falco --set falcosidekick.enabled=true --set falcosidekick.webui.enabled=true --set ebpf.enabled=true
```

After that, you just need to verify if all Pods in namespace `falco` are running, with `kubectl get pods -n falco`

With this install, I've also enabled [sidekick](https://github.com/falcosecurity/falcosidekick) which is a really cool project that exports Falco events to a bunch of outputs, and also allows you to visualize them in its own interface

## Creating and visualizing alerts

Falco has some default rules. As an example, as soon as it is executing, if you issue a `kubectl exec -it` to a Pod, it will generate some alert:

```shell
kubectl exec -it -n testkatz nginx-6799fc88d8-996gz -- /bin/bash
root@nginx-6799fc88d8-996gz:/#
```

And in Falco logs (I've made this JSON clearer, but it's a one liner)
```json
{
    "output":"14:23:15.139384666: Notice A shell was spawned in a container with an attached terminal (user=root user_loginuid=-1 k8s.ns=testkatz k8s.pod=nginx-6799fc88d8-996gz container=3db00b476ee2 shell=bash parent=runc cmdline=bash terminal=34816 container_id=3db00b476ee2 image=nginx) k8s.ns=testkatz k8s.pod=nginx-6799fc88d8-996gz container=3db00b476ee2",
    "priority":"Notice",
    "rule":"Terminal shell in container",
    "time":"2021-04-16T14:23:15.139384666Z",
    "output_fields": {
        "container.id":"3db00b476ee2",
        "container.image.repository":"nginx",
        "evt.time":1618582995139384666,
        "k8s.ns.name":"testkatz",
        "k8s.pod.name":"nginx-6799fc88d8-996gz",
        "proc.cmdline":"bash",
        "proc.name":"bash",
        "proc.pname":"runc",
        "proc.tty":34816,
        "user.loginuid":-1,
        "user.name":"root"
    }
}
```
In this log you can check that besides the output message, some additional fields are mapped, like the process name, the namespace and the pod name. We will explore this further.


## Creating rules for Falco

Falco can monitor syscalls. A syscall of a network connection is of the 'connect' type, and this way, we can create a basic rule so Falco can always generate a notification when some container tries to connect to the outside world.

Falco already comes with a pre defined list and macro for outbound connections:

```yaml
# RFC1918 addresses were assigned for private network usage
- list: rfc_1918_addresses
  items: ['"10.0.0.0/8"', '"172.16.0.0/12"', '"192.168.0.0/16"']

- macro: outbound
  condition: >
    (((evt.type = connect and evt.dir=<) or
      (evt.type in (sendto,sendmsg) and evt.dir=< and
       fd.l4proto != tcp and fd.connected=false and fd.name_changed=true)) and
     (fd.typechar = 4 or fd.typechar = 6) and
     (fd.ip != "0.0.0.0" and fd.net != "127.0.0.0/8" and not fd.snet in (rfc_1918_addresses)) and
     (evt.rawres >= 0 or evt.res = EINPROGRESS))
```

This macro:
* Verifies if the event is of type connect (network connection) and it's outbound (`evt.dir=<`)
* OR if this is a sendto or sendmsg type going outbound, which protocol is not TCP and the file descriptor is not connected, and the name does not change, tipically an UDP connection.
* If any of the conditions above are true AND the file descriptor is 4 or 6, which represents IPv4 or IPv6
* AND the IP is not equal to  0.0.0.0 AND the network is not equal to 127.0.0.0/8 (localhost) AND the destination network (`fd.snet`) is not in list `rfc_1918_addresses`
* AND the return from the event is bigger than 0 OR is INPROGRESS

If you put this expression in vscode, you can work through the openning and closure of parenthisis. All existing fields are really well explained in [supported fields](https://falco.org/docs/rules/supported-fields)

But the above macro does not fits our needs, as it ignores the outbound connections to internal networks (rfc1918), which is the majority of enterprise cases. Let's define then our own macro and rule:

```yaml
- macro: outbound_corp
  condition: >
    (((evt.type = connect and evt.dir=<) or
      (evt.type in (sendto,sendmsg) and evt.dir=< and
       fd.l4proto != tcp and fd.connected=false and fd.name_changed=true)) and
     (fd.typechar = 4 or fd.typechar = 6) and
     (fd.ip != "0.0.0.0" and fd.net != "127.0.0.0/8") and
     (evt.rawres >= 0 or evt.res = EINPROGRESS))

- list: k8s_not_monitored
  items: ['"green"', '"blue"']
  
- rule: kubernetes outbound connection
  desc: A pod in namespace attempted to connect to the outer world
  condition: outbound_corp and k8s.ns.name != "" and not k8s.ns.label.network in (k8s_not_monitored)
  output: "Outbound network traffic connection from a Pod: (pod=%k8s.pod.name namespace=%k8s.ns.name srcip=%fd.cip dstip=%fd.sip dstport=%fd.sport proto=%fd.l4proto procname=%proc.name)"
  priority: WARNING
```
The rule above:
* Creates a macro `outbound_corp` that deals with any outbound connection
* Creates a list `k8s_not_monitored` with values `blue` and `green`
* Creates a rule that verifies:
  * If it's an outbound traffic defined in macro `outbound_corp`
  * AND If the field k8s.ns.name is defined (which means it's being executed inside Kubernetes)
  * And if the namespace containing the Pod does not have a label `network` containing any of the values in list `k8s_not_monitored`. If it does, the traffic wont be monitored

When this rule is triggered, the following output might be seen: 
```
Outbound network traffic connection from a Pod: (pod=nginx namespace=testkatz srcip=172.16.204.12 dstip=192.168.0.1 dstport=80 proto=tcp procname=curl)
```

## Applying the rules

As we've installed Falco using a helm chart, its rules configurations is a `ConfigMap` in falco namespace.

Before we edit and mess with anything, a backup must be made:

```shell
kubectl get cm -n falco falco -o yaml > falcoorig.yaml
cp falcoorig.yaml falco.yaml
```

Now, inside the file `falco.yaml` let's edit the field `falco_rules.local.yaml` (and only it!) to add our new set of rules defined above.

**Obs**: There's for sure a less dumb way of doing this, if you know how, call me on Twitter or Slack and give me an idea on how to patch only the `falco_rules.local.yaml` part :D

After editing the file, let's just delete and re-create (because some weird things happens) and then re-start falco deleting its Pods:

```shell
kubectl delete -n falco -f falco.yaml
kubectl create -n falco -f falco.yaml
sleep 2
kubectl delete pod -n falco -l app=falco
```

If the Pod enters in an `Error` state, take a look into their logs to see if there's some failure on rules (yaml = space problems!)

## Show me the logs!!!

Executing `kubectl logs -n falco -l app=falco` we will see our outbound connection logs appearing:

```json
{
    "output":"18:05:13.045457220: Warning Outbound network traffic connection from a Pod: (pod=falco-l8xmm namespace=falco srcip=192.168.0.150 dstip=192.168.0.11 dstport=2801 proto=tcp procname=falco) k8s.ns=falco k8s.pod=falco-l8xmm container=cb86ca8afdaa",
    "priority":"Warning",
    "rule":"kubernetes outbound connection",
    "time":"2021-04-16T18:05:13.045457220Z", 
    "output_fields": 
    {
        "container.id":"cb86ca8afdaa",
        "evt.time":1618596313045457220,
        "fd.cip":"192.168.0.150",
        "fd.l4proto":"tcp",
        "fd.sip":"192.168.0.11",
        "fd.sport":2801,
        "k8s.ns.name":"falco",
        "k8s.pod.name":"falco-l8xmm"
    }
}
```
But those are logs generated by the own Falco containers, and we don't want them. Let's mark this namespace with the label that will make it stop generating the logs from those Pods traffic:

```shell
kubectl label ns falco network=green
```

Nice! Now that Falco Pods does not enter anymore in monitoring, let's make some tests :D For that, I've created a namespace called `testkatz` with some Pods inside, and then I've started to generate some outbound traffic:


```log
"output":"18:11:04.365837060: Warning Outbound network traffic connection from a Pod: (pod=nginx-6799fc88d8-996gz namespace=testkatz srcip=172.16.166.174 dstip=10.96.0.10 dstport=53 proto=udp procname=curl)
=====
"output":"18:11:04.406290360: Warning Outbound network traffic connection from a Pod: (pod=nginx-6799fc88d8-996gz namespace=testkatz srcip=172.16.166.174 dstip=172.217.30.164 dstport=80 proto=tcp procname=curl)
```

In the above log, we can see a call to the DNS, followed by a call to the destination server. We can see also which program inside the container started this traffic.

## A better visualization

No one deserves to monitor by watching JSON logs streaming on the screen, right? Here comes Falco Sidekick to the rescue. It was installed with Falco, so we only need to configure it to send those "alerts" to a desired output.

Sidekick comes with a web interface, that can be accesses with a port-forward, as example:

```
kubectl port-forward -n falco pod/falco-falcosidekick-ui-764f5f469f-njppj 2802
```

After that, you only need to access with your browser  [http://localhost:2802/ui] and you will have something as cool as:

![Sidekick Dashboard](/images/falcomonitoring/sidekick1.png)

![Sidekick Events](/images/falcomonitoring/sidekick2.png)

But I want to send this to a place where I can retain this, as some install of Grafana Loki. You can use the Grafana Cloud Freetier for this example, but do not use this in production if you have a lot of logs :) 

After generating a user and apikey in Grafana Cloud, you can update your Falco sidekick install with the following command (thanks Thomas Labarussias for the idea!):

```
helm upgrade falco falcosecurity/falco --namespace falco --set falcosidekick.enabled=true --set falcosidekick.webui.enabled=true --set ebpf.enabled=true --set falcosidekick.config.loki.hostport=https://USER:APIKEY@logs-prod-us-central1.grafana.net
```

Restart sidekick with `kubectl delete pods -n falco -l app.kubernetes.io/name=falcosidekick` and you should see messages like `[INFO]  : Loki - Post OK (204)` in sidekick log each time a new alert is triggered. 

With that, you may have a dashboard in Grafana Cloud as I shown you on the beginning of this article :)

[Here](https://gist.github.com/rikatz/d53751acf9b705262db992b3bd98acbe#file-dashboard-json) is my dashboard example, but remember to change your Loki datastore, and also if you have improved this dashboard please post its config and a picture to make me happy! 
