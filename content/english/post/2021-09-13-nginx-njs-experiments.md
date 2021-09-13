---
title: "NGINX NJS Experiments - Dynamic Backends"
date: 2021-09-12T23:00:00-03:00
draft: false
description: "Exploring NGINX NJS Dynamic Backends and some fun"
featured: true # Sets if post is a featured post, making appear on the home page side bar.
toc: true 
codeMaxLines: 100 # Override global value for how many lines within a code block before auto-collapsing.
codeLineNumbers: true # Override global value for showing of line numbers within code block.
figurePositionShow: true # Override global value for showing the figure label.
categories:
  - Technology
tags:
  - NGINX
  - Ingress

---

# NGINX NJS and the dynamic config

Recently, due to some bugs in Ingress NGINX I have decided to look into alternatives on the dynamic upstream configuration.

For those not familiar with Ingress NGINX dynamic configuration, it relies massively on Lua and [Openresty components](https://github.com/openresty/lua-nginx-module)  to allow the reconfiguration of backends without restarting NGINX process, for example.

More and more features have been added to the Lua/openresty side of Ingress due to how "easy" it is to implement, for example different load balancing alghorithms, canary balancing and even global rate limit. I'm pretty null at Lua coding, but other Ingress NGINX maintainers have been doing that pretty well!

So, I decided to look to alternatives. Not because Openresty and Lua are bad technologies. Opposite to that, those are pretty mature and stable technologies, and widely used in cool things like Kong API Gateway (https://konghq.com/kong/), Curiefense WAF (https://www.curiefense.io/) and also as Envoy and HAProxy filters. 

The reason I decided to do that is a simple question I did to myself: "can we do better? Can we improve this somehow? What can I learn from this?"

For this article, I will reproduce the most used Lua part of Ingress NGINX: dynamic upstreams. Not as complete as the real one, or with all features, but just for fun.

I will put all of the experiments here in https://github.com/rikatz/njs-experiments

# NGINX NJS scripting language
Back to 2015 (I guess…) NGINX Inc announced a new feature in NGINX, called ngiScript, which turned later into NJS. NJS allows one to write the behavior of NGINX based on Javascript language.

It's not as wide as the whole Javascript language but still allows one to do some cool things. There are some few examples [here](https://nginx.org/en/docs/njs/examples.html) and [here](https://github.com/xeioex/njs-examples) like creating some JWT signing or even an [S3 Gateway'ish](https://github.com/nginxinc/nginx-s3-gateway/). You may find more examples in https://www.nginx.com/blog/tag/nginx-javascript-module/ and also googling around there about people using NJS as an antispam bot, etc.

Overall, you need to think on NJS by some perspectives:
* It is NOT as complete as Lua. You can vendor some external [nodejs modules](https://nginx.org/en/docs/njs/node_modules.html) but do you really wanna download half the internet to process something? (npm joke here...sorry…)
* For ever request that your server needs to deal with, it creates a small thread to run your javascript. It is pretty fast, but still you may end adding some latency in the processing of your requests. I haven't tested it (yet!) to see the performance differences between Lua x NJS.
* NJS is a core module from NGINX. While to make Lua work you need some Openresty external stuff, LuaJIT and other things that can go wrong, NJS is part of Nginx code. It's not installed as default, but you can simple download it from official NGINX package repo (or compile), and its version relates to the version you are using in NGINX (while with Openresty you are sort of stuck with previous versions, like v1.19, although we use it with NGINX v1.20 in Ingress NGINX).

See more here: https://www.nginx.com/blog/harnessing-power-convenience-of-javascript-for-each-request-with-nginx-javascript-module/

# Starting simple

So my first experiment: **The hello world** example.

Because I'm too lazy to install a new machine and do all the required things, I've just used the "nginx:stable" docker image that have everything built in.

So, imagine that this is my current filesystem:
* etc/nginx.conf -> Where nginx conf lives
* njs/ -> Where my njs scripts live

My first approach is:

* etc/nginx.conf
```
load_module modules/ngx_http_js_module.so;
events {}
http {
    js_import /etc/nginx/njs/http.js;
    server {
      listen 80;
      location / {
        js_content http.hello;
      }
    }
}
```
* njs/http.js
```javascript
function hello(r) {
    r.error(JSON.stringify(r));
    r.return(200, "Hello world!");
}
export default {hello};
```

Not getting too deep into Javascript, but basically this:
* Will import the njs/http.js as a module
* Call the function http.hello every time someone calls our server on port 80 and location /
* Will log as an error (just for sake of debugging) the content of our request in servers log
* Will return a `200-OK Hello world` to our browser

So, running this container as:
```
docker run -it -p 18080:80 -v $PWD/etc/nginx.conf:/etc/nginx/nginx.conf -v $PWD/njs/:/etc/nginx/njs/ --rm nginx:stable
```
And calling it with `curl http://127.0.0.1:18080/lalala?test=xpto` we get a Hello world, and the following log:
```
2021/09/12 23:59:16 [error] 32#32: *1 js: {"status":0,"args":{"test":"xpto"},"httpVersion":"1.1","remoteAddress":"172.17.0.1","headersOut":{},"method":"GET","uri":"/lalala","headersIn":{"Host":"127.0.0.1:18080","User-Agent":"curl/7.74.0","Accept":"*/*"}}
```

Nice uh. So, by the log, we know that there is a bunch of stuff we can extract from our requests (see the Reference https://nginx.org/en/docs/njs/reference.html)

# Doing some dynamic backend experiments

So now we know that we can extract Headers and URI with Javascript, which are two important parts that can help us on the routing decision.

Let's then write a new Javascript that returns an upstream based on the Host information:

* njs/perhost.js
```javascript
function getUpstream(req) {
    var host;
    host = req.headersIn['Host'].toLowerCase();
    if (host == "mydomain1.com") {
        return "https://www.google.com"
    }
    if (host == "mydomain2.com") {
        return "https://www.amazon.com"
    }
    req.return(404, "Backend not found");
    req.finish();
    // Invalid return just so it wont complain
    return "@invalidstuff"
}

export default {getUpstream};
```
* etc/nginx.conf
```
load_module modules/ngx_http_js_module.so;
events {}
http {
    resolver 8.8.8.8;
    js_import "/etc/nginx/njs/perhost.js";
    js_set $upstream perhost.getUpstream;
    server {
      listen 80;
      location / {
        proxy_pass $upstream;
      }
    }
}
```

Doing some tests now, we can see that:
```
curl -H "mydomain1.com" http://127.0.0.1:18080 # Returns Google
curl -H "mydomain2.com" http://127.0.0.1:18080 # Returns Amazon
curl -H "somethingweird.com" http://127.0.0.1:18080 # Returns 404 Backend not found
``` 

Before someone complains that I'm not checking if Host is null, this is intentional. If you call our Nginx without the host header the user will receive a 500 as the Host field does not exists and we still try to convert it to lowercase. It's good to know that only that Javascript VM dies, but still in prod scripts this needs to be checked properly!

# Going far and reading the upstreams config from somewhere else

Until now we could play with NJS and have some dynamic responses based on headers. But so far, everything is still pretty "static", as the decision making is hardcoded into our Javascript.

But how to do something near to the real world, with dynamic reconfigurations? In case of Ingress NGINX, Lua script is a long running program that listens on a specific port and gets configuration updates from Ingress Controller, adding the endpoints into a local map.

In NJS, as each script runs and dies per request it does not seem possible to store a cache/map to be used with endpoints, so what's the easiest way? Files containing endpoints :)

I will not dive here into performance implications, but only on the viability of this. For this solution we want to follow the current approach of Ingress NGINX:

* "set" a variable called "proxy_upstream_name" containing a "namespace-servicename-port"
* have a file in a directory called "upstreams/namespace-servicename-port" containing one upstream per line
```
www.google.com:80
www.amazon.com:443
192.168.0.10:9999
```
* For each request, we get the variable name from the request, open the correct file (if it exists), select one random upstream.

Obs.: Ingress NGINX nowadays reloads the config if backend protocol or locations are changed. We won't mess with this in this article, but this might be an improvement when using NJS: We can put all of the other configurations as dynamic files, and simply read from them.

So here is the implementation:

* etc/nginx.conf
etc/nginx.conf
```
load_module modules/ngx_http_js_module.so;
events {}
http {
    resolver 8.8.8.8;
    js_import /etc/nginx/njs/ingress.js;
    js_set $upstream ingress.getUpstream;
    server {
      listen 80;
      location / {
        set $ingress_service "default-nginx1-80";
        proxy_pass http://$upstream;
      }
      location /otherlocation {
        set $ingress_service "rkatz-nginx2-80";
        proxy_pass http://$upstream;
      }
      location /otherlocation1 {
        proxy_pass http://$upstream;
      }
    }
}
```

* njs/ingress.js
```javascript
function getUpstream(req) {
    var fs = require('fs')
    var upstreamfile;
    var upstream;
    var service;
    if ("variables" in req) {
      service = req.variables.ingress_service;
      if (service == "") {
        return invalidBackend(req, 404);
      }
    }

    upstreamfile = "/etc/nginx/upstreams/" + service;
    try {
        upstream = fs.readFileSync(upstreamfile, 'utf8');
        var endpointArr = upstream.split("\n");
        if (endpointArr.length == 1) {
            if (endpointArr[0] != "") {
                return endpointArr[0].replace(/^\s+|\s+$/g, '');
                }
            return invalidBackend(req, 404);
            }
        var randomBackend = Math.floor(Math.random() * (endpointArr.length - 1));
        // The final replace is to remove some dirty line break
        return endpointArr[randomBackend].replace(/^\s+|\s+$/g, '');

    } catch (e) {
        req.error(e)
        return invalidBackend(req, 502);
        }
    return invalidBackend(req, 503);
}

function invalidBackend(req, code) {
    req.return(code, "Invalid Backend");
    req.finish();
    return "@invalidbackend"
}

export default {getUpstream};
```
This is getting more complex, but pretty easy to read: "we will use the variable `ingress_service` that is set in nginx.conf, to discover what file should be read. So, if `ingress_service=rkatz-myapp-80` a file with this name must exist in `/etc/nginx/upstreams`.

And here are the upstream data:
* rkatz-nginx2-80
```
www.google.com
www.amazon.com
www.uol.com.br
192.168.0.10
```
* default-nginx1-80
```
www.rkatz.xyz
```

Now calling myserver:80/ will proxy to www.rkatz.xyz, while calling myserver:80/otherlocation will randomize calls between the endpoints in file `rkatz-nginx2-80`.

If I call myserver:80/otherlocation1, I will end up with an error of `Invalid backend` as no `ingress_service` variable is being set for that location.

Oh, and if we change the content of the files, it can dinamically point to different backends. No need to restart stuff :)

# Some conclusions

NJS have a lot of potential to offload some dynamic reconfiguration needs in Ingress NGINX and other services.

While it still have some concerns about the community, the features are pretty nice and easy to develop, using common javascript and can also be used to implement some "FaaS" on the edge, for example.