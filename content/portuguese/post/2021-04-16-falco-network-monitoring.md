---
title: "Usando o Falco para monitorar o tráfego de Pods no Kubernetes"
date: 2021-04-16T16:32:27-03:00
draft: false
description: "Como usar o Falco para monitorar o tráfego de rede de seus Pods" 
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

# Usando o Falco para monitorar o tráfego de Pods no Kubernetes

O [Falco](https://www.falco.org) é um projeto opensource da [Sysdig](https://sysdig.com/) para segurança de ambientes Cloud Native, que utiliza-se de tecnologias modernas como [eBPF](https://ebpf.io) para monitorar situações no ambiente através de [syscalls](https://man7.org/linux/man-pages/man2/syscalls.2.html) e outros geradores de [eventos](https://falco.org/docs/event-sources/)

Nós já usamos o Falco a algum tempo atrás no trabalho como uma PoC para monitorar alguns eventos específicos, e recentemente eu comecei a mexer com o para ajudar a contribuir a pedido do grande [Dan Pop](https://twitter.com/danpopnyc/) e também para entender melhor como o Falco poderia me ajudar em situações futuras (bem como entender o seu funcionamento).

## O Problema que eu queria resolver

Quem mexe com Kubernetes sabe o quão difícil é manter o rastreio de conexões originadas de seus Pods. Mesmo que você coloque [Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/), é muito difícil monitorar que Pod tentou abrir uma conexão para o mundo exterior.

Ter esse tipo de informação é fundamental em ambientes com um nível de restrição maior, ou que você precise rapidamente responder 'qual Pod conectou-se a esse outro servidor', inclusive por motivos legais. 

Alguns CNIs provêm uma solução, como o [Antrea](https://antrea.io) exportando o fluxo de dados via [Netflow](https://antrea.io/docs/v1.0.0/network-flow-visibility/), ou o [Cilium](https://cilium.io/) com uma solução de curta retenção através do seu projeto [Hubble](https://github.com/cilium/hubble).

Mas eu queria algo mais: Eu queria uma solução que se aplique a qualquer CNI. Se a conexão é dropada ou não, não me importa, mas como toda conexão originada de um container gera uma syscall de conexão, como eu poderia monitorar isso e cruzar com as informações do Kubernetes?

## O resultado final

![Resultado Final](/images/falcomonitoring/grafana.png)

## Te apresento o Falco!

Conforme expliquei anteriormente, o Falco é um projeto Opensource que monitora eventos no servidor (que podem ser eventos de auditoria do Kubernetes, ou até syscalls de containers executando em um host).

O Falco é baseado em [regras](https://falco.org/docs/rules/). Essas regras definem principalmente:
* um **nome** comum (`rule`) - "Acesso indevido via SSH"
* uma **descrição** (`desc`) - "Um usuário tentou efetuar um login via SSH no servidor"
* uma **prioridade** (`priority`) - "WARNING"
* Uma **saída detalhada** do evento (`output`) - "O servidor 'homer' recebeu uma conexão na porta 22 vinda do IP não autorizado 192.168.0.123"
* uma **CONDIÇÃO** (`cond`) - "(O servidor tiver no seu hostname a palavra 'restrito' e receber uma conexão na porta 22 que não venha da rede 10.10.10.0/24) OU (O servidor recebeu uma conexão na porta 22 mas o processo que abriu essa conexão não se chama 'sshd')"

Aqui cabe uma observação: Eu escrevi a condição de uma forma 'simples' em linguagem comum. Apesar de as regras do Falco não serem escritas dessa forma (veremos mais adiante), o formato é muito 'similar' tornando a leitura das regras bem tranquila!

As regras podem conter também outros campos não explicados aqui (exceções, tags adicionais, etc) bem como  **Listas** e **Macros** que facilitam quando alguma condição é repetitiva (irei mostrar adiante).

## Instalando o Falco

Antes de começar a instalar o Falco, apenas uma descrição de meu ambiente:

* 3 servidores virtuais com o [Flatcar Linux](https://kinvolk.io/flatcar-container-linux/) 2765.2.2, porque EU GOSTO MUITO DO MODELO DO FLATCAR!! (fora que ele já tem um Kernel mais moderno e usar o Falco no modo eBPF simplesmente funciona!!). Se você quiser aprender a instalar o Flatcar em 5 minutos no VMware Player, tem um outro artigo explicando aqui no blog.
* Meus servidores e minha rede aqui em casa são `192.168.0.0/24`
* Meu Kubernetes é o v1.21.0, instalado via Kubeadm. Os pods são criados na rede `172.16.0.0/16`

Para instalar o Falco no Kubernetes, basicamente são 4 passos (claro que você pode alterar, eu segui assim para demonstrar aqui):

```bash=
kubectl create ns falco # Eu quero rodar em outro namespace
helm repo add falcosecurity https://falcosecurity.github.io/charts
helm repo update
helm install falco falcosecurity/falco --namespace falco --set falcosidekick.enabled=true --set falcosidekick.webui.enabled=true --set ebpf.enabled=true
```

Após isso, basta verificar se todos os Pods no namespace `falco` estão executando com `kubectl get pods -n falco`

Na instalação acima, eu habilitei também o [sidekick](https://github.com/falcosecurity/falcosidekick) que é um projeto bem maneiro para exportar os eventos do Falco (usarei para exportar para um Elasticsearch), bem como visualizar esses eventos em sua própria UI.

## Gerando alertas e visualizando

O Falco vem com algumas regras por padrão. Por exemplo, à partir do momento que ele está em execução, se você tentar dar um `kubectl exec -it` em um Pod, ele irá gerar um alerta:

```shell=zsh
controlplane #  kubectl exec -it -n testkatz nginx-6799fc88d8-996gz -- /bin/bash
root@nginx-6799fc88d8-996gz:/#
```

E nos logs do Falco (eu dei uma melhorada nesse JSON!):
```json=
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
Você pode ver nesse log, por exemplo que além da mensagem no output, alguns campos adicionais foram mapeados (como o nome do processo, a namespace, o nome do Pod). Iremos explorar mais isso à seguir.

## Criando regras para o Falco

Como eu falei, o Falco consegue monitorar as chamadas em syscall. Uma syscall de conexão de rede é do tipo 'connect', e sendo assim, podemos criar uma regra básica para o Falco que sempre gere uma notificação quando algum container tentar conectar-se a algum ativo externo a ele. 

O Falco vem com uma macro e uma lista pré definida para conexões que saiam de rede assim:
```
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

Essa Macro:
* Verifica se é um evento do tipo connect (conexão de rede) do tipo saída (evt.dir=<)
* OU se é um evento do tipo sendto, sendmsg (conexão via socket de arquivo) do tipo saída, que o protocólo não seja TCP, o filedescriptor não esteja conectado e o nome mude, tipicamente de conexões UDP
* Caso alguma das condições acima seja verdadeira, e o tipo do file descriptor seja 4 ou 6 (IPv4 ou IPv6)
* E o IP não seja igual a 0.0.0.0 E a rede não seja igual a 127.0.0.0/8 (conexão ao localhost) E a rede de destino (fd.snet) não esteja na lista `rfc_1918_addresses`
* E o evento tenha retorno maior que zero ou esteja 'Em andamento'

Difícil? Na verdade, jogue essa expressão no vscode, e vá trabalhando ela através da abertura e fechamento de parenteses. Todos os campos existentes aqui estão muito bem explicados em [Campos suportados](https://falco.org/docs/rules/supported-fields)

Porém, a macro acima não nos atende, pois filtra a saída para redes internas (rfc1918), o que é o caso da maioria das empresas. Vamos definir então o nosso conjunto macro/regra:

```
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
  condition: outbound_corp and k8s.ns.name != "" and k8s.ns.name != "falco" and not k8s.ns.label.network in (k8s_not_monitored)
  output: "Outbound network traffic connection from a Pod: (pod=%k8s.pod.name namespace=%k8s.ns.name srcip=%fd.cip dstip=%fd.sip dstport=%fd.sport proto=%fd.l4proto procname=%proc.name)"
  priority: WARNING
```

A regra acima:
* Cria uma macro `outbound_corp` para qualquer conexão saindo
* Cria uma lista `k8s_not_monitored` com os valores `blue` e `green`
* Cria uma regra que verifica:
  * Se é tráfego saindo (`outbound`)
  * E se tem o campo k8s.ns.name definido (o que significa que está executando dentro do Kubernetes)
  * E se o namespace NÃO TEM uma label chamada `network` com algum dos valores que está na lista `k8s_not_monitored`. Se tiver, o tráfego não será monitorado

Se essa regra for acionada, a seguinte saída será gerada: 
```
Outbound network traffic connection from a Pod: (pod=nginx namespace=testkatz srcip=172.16.204.12 dstip=192.168.0.1 dstport=80 proto=tcp procname=curl)
```

## Aplicando as regras

Como instalamos o Falco como um helm chart, sua configuração de regras é um `ConfigMap` no namespace falco.

Antes de editarmos e fazermos qualquer besteira, o ideal é fazer um backup desse arquivo:

```
kubectl get cm -n falco falco -o yaml > falcoorig.yaml
cp falcoorig.yaml falco.yaml
```

Agora, dentro desse configmap vamos editar o arquivo falco_rules.local.yaml (e apenas ele!) para adicionarmos nosso novo conjunto de regras definido acima.

**Obs**: Com certeza existe algum jeito menos idiota de fazer isso, se você souber como, me chama no twitter e fala como seria um patch nesse ConfigMap aí só com o `falco_rules.local.yaml`

Depois de editar o falco.yaml, basta reaplicar o ConfigMap e apagar todos os Pods do Falco, que serão re-criados:

```
kubectl replace -n falco -f falco.yaml
sleep 2
kubectl delete pod -n falco -l app=falco
```

Se o Pod estiver em status de Error, veja o log deles que podem indicar alguma falha na regra.

## Mostre-me os Logs!!

Executando um `kubectl logs -n falco -l app=falco` veremos que nossos logs já estão aparecendo:

```
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
Mas esses são os Logs do próprio Falco, que não nos interessam. Vamos então marcar o namespace dele com a label 'green' e ele irá parar de gerar logs do tráfego dos Pods do Falco:

```
kubectl label ns falco network=green
```

Show, agora que os Pods do Falco não entram nos logs de monitoração, vamos fazer uns testes :D Para isso eu criei um namespace chamado `testkatz` com alguns Pods dentro, e irei gerar logs:

```
"output":"18:11:04.365837060: Warning Outbound network traffic connection from a Pod: (pod=nginx-6799fc88d8-996gz namespace=testkatz srcip=172.16.166.174 dstip=10.96.0.10 dstport=53 proto=udp procname=curl)
=====
"output":"18:11:04.406290360: Warning Outbound network traffic connection from a Pod: (pod=nginx-6799fc88d8-996gz namespace=testkatz srcip=172.16.166.174 dstip=172.217.30.164 dstport=80 proto=tcp procname=curl)
```

No log acima, podemos ver a chamada ao DNS, e em seguida a chamada ao servidor de destino.  Vemos inclusive que o programa que fez essas chamadas foi o `curl` rodando dentro do container.

## Visualizando de forma melhorada

Ninguém merece ficar vendo logs JSON rodando na tela, certo? E se eu quisesse gerar alertas desses logs, por exemplo?

Entra em ação aqui o falcosidekick. Ele foi instalado no começo, então a configuração que precisamos fazer é para que ele envie esses "alertas" para algum lugar que gere uma visualização melhorada.

O sidekick por padrão vem com uma interface Web, que pode ser acessada via port-forward, por exemplo:
```
kubectl port-forward -n falco pod/falco-falcosidekick-ui-764f5f469f-njppj 2802
```

Após isso, basta acessar o browser local e você terá algo tão legal quanto:

![Sidekick Dashboard](/images/falcomonitoring/sidekick1.png)

![Sidekick Events](/images/falcomonitoring/sidekick2.png)

Mas isso não gera retenção. Vamos então configurar o Sidekick para apontar para um Grafana Loki. Pode-se usar o freetier do Grafana Cloud, por exemplo. Gere um hash base64 da URL do Loki, conforme a seguir:

```
echo -n "https://USER:APIKEY@logs-prod-us-central1.grafana.net" |base64 -w0
```

Com o hash base64 gerado, adicione ele na secret falco/falcosidekick na linha `LOKI_HOSTPORT`

Por fim, reinicie o sidekick com `kubectl delete pods -n falco -l app.kubernetes.io/name=falcosidekick` e então você passará a ver mensagens `[INFO]  : Loki - Post OK (204)` cada vez que uma nova mensagem for postada.

Com isso, no Grafana Cloud você conseguirá ter um dashboard parecido com o que mostrei acima :)

Eu deixei meu exemplo de dashboard [aqui](https://gist.github.com/rikatz/d53751acf9b705262db992b3bd98acbe#file-dashboard-json) mas lembre-se de alterar para o seu datasource do Loki e, caso você tenha melhorado o dashboard, não deixe de postar e mandar uma foto :D
