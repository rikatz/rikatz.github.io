---
title: "Sobre o KYAML e por que você deveria agradecer por essa nova feature"
date: 2025-08-29T09:34:00-03:00
draft: false
description: "Um deep dive no KYAML, o motivo de ele ter sido criado, o que vai ajudar a melhorar no Kubernetes, etc" 
featured: true # Sets if post is a featured post, making appear on the home page side bar.
toc: true 
codeMaxLines: 100 # Override global value for how many lines within a code block before auto-collapsing.
codeLineNumbers: true # Override global value for showing of line numbers within code block.
figurePositionShow: true # Override global value for showing the figure label.
categories:
  - Technology
tags:
  - Kubernetes
---

## Sobre o KYAML e por que você deveria agradecer por essa nova feature

A versão 1.34 do Kubernetes anunciou uma nova feature que gerou bastante polêmica:
o KYAML (Kubernetes YAML).

Após esse anúncio eu notei diversos comentários como:
* Puxa, mais um formato diferente?
* Não era melhor ter usado CUE?
* YAML já era ruim, JSON com YAML então, o que os mantenedores tem na cabeça?
* Eu estou curioso pra saber o que justificou isso.
* Vai quebrar um monte de coisa

Nesse artigo eu vou tentar responder a cada uma dessas perguntas demonstrando como 
o KYAML na realidade veio pra ajudar e sem causar o impacto que se imagina, e na 
realidade sendo uma nova funcionalidade muito bem vinda para corrigir problemas 
históricos do YAML sem quebrar a retrocompatibilidade.

## Um pouco de contexto

O YAML foi introduzido no Kubernetes em sua primeira versão como uma forma de declarar 
os seus recursos de uma forma que fosse legível para seres humanos, extendendo deficiências do JSON 
como:
* Complexidade (lembre que JSON não precisa de quebra de linha, por exemplo)
* Impossibilidade de adicionar comentários
* Necessidade de terminar determinadas linhas com vírgulas, lembrar de colocar aspas, etc.

Na prática, JSON é muito bonito para máquinas, mas não é intuitivo para humanos. A realidade 
é que nós adoramos falar mau do YAML (afinal, malditos espaços vs tabs) mas poderia ser pior :)

O Joe Beda, um dos criadores do Kubernetes deu uma palestra sobre a decisão do uso do YAML 
e eu recomendo demais assistir ela [aqui](https://www.youtube.com/watch?v=8PpgqEqkQWA). É 
uma palestra de 30 minutos engraçada e esclarecedora.

## Os problemas do YAML e a justificativa para o KYAML

Se você quiser ir direto ao ponto, sugiro ler o [Kubernetes Enhancement Proposal](https://github.com/kubernetes/enhancements/tree/master/keps/sig-cli/5295-kyaml)
que explica todo o design do KYAML.

Antes de mais nada, eu vou reforçar diversas vezes aqui: **O KYAML É APENAS UM 
NOVO TIPO DE OUTPUT NO KUBECTL**

Vou demonstrar depois como nada muda no lado do servidor, mas sem spoilers por enquanto.

Os exemplos à seguir foram executados em um cluster KinD (https://kind.sigs.k8s.io/docs/user/quick-start/) e o 
[CRD](/demo/kyaml/myapi.yaml) gerado à partir da [spec](/demo/kyaml/api.go).

```golang
// MyAPISpec contains the details of the best API in the world
type MyAPISpec struct {
	// +optional
	MyField string `json:"myField"`
	// +optional
	MyArray []string `json:"myArray"`
}
```

Para início, vamos aplicar esse CRD em nosso cluster criar um objeto simples (baixe o CRD do link acima!)
```shell
kubectl apply -f crd/myapi.yaml
```

### Quando uma string não é uma string
Com o CRD instalado, vamos criar um objeto simples no Kubernetes:
```shell
kubectl apply -f - <<EOF
apiVersion: home.lab/v1
kind: MyAPI
metadata:
  name: not-a-bool
spec:
  myField: yes
EOF
```
E o seguinte erro irá acontecer:
`The MyAPI "not-a-bool" is invalid: spec.myField: Invalid value: "boolean": spec.myField in body must be of type string: "boolean"`

Mas o que aconteceu? O campo não era uma string? E booleano não é `true/false`? Pois é, 
acontece que yes/no, on/off também são considerados booleanos no caso do YAML. Isso causa 
uma situação chamada `The Norway Bug`. Imagina que o campo `myField` receba o código de 
um país: para `Brasil` teríamos `BR`, para `Argentina` teríamos `AR`, e para `Noruega` teríamos `NO`, 
mas para o YAML, "NO" é equivalente a Falso, e eis o erro.

Mas aí você pensa, se eu fizer algo como à seguir vai funcionar:
```shell
kubectl apply -f - <<EOF
apiVersion: home.lab/v1
kind: MyAPI
metadata:
  name: not-a-bool
spec:
  myField: "yes"
EOF
kubectl get myapi not-a-bool -o yaml
....
spec:
  myField: "yes"
```

Sim! E isso funciona pois como essas situações são conhecidas, o printer do kubectl 
já tem um "best effort" de colocar essas strings específicas entre aspas, mas vamos ver um outro exemplo:

```shell
kubectl apply -f - <<EOF
apiVersion: home.lab/v1
kind: MyAPI
metadata:
  name: maybe-a-bool
spec:
  myArray:
  - "yes"
  - "maybe"
EOF
kubectl get myapi maybe-a-bool -o yaml
....
spec:
  myArray:
  - "yes"
  - maybe
```

E la se foi a consistência. Para onde foram as aspas do ítem `"maybe"`? Assim, o printer 
KYAML vai sempre adicionar aspas em strings, mesmo que isso fique mais verboso (e existe 
uma explicação para isso: retrocompatibilidade!)
```yaml
spec: {
    myArray: [
      "yes",
      "maybe",
    ],
  },
```

### Quando uma string tem várias linhas
Esse caso é peculiar. Você vai lá, escreve seu yaml bonitinho, precisa adicionar uma linha a mais 
e quando vai dar um get nele vem tudo errado. Por exemplo:

```shell
kubectl apply -f - <<EOF
apiVersion: home.lab/v1
kind: MyAPI
metadata:
  name: multiline1
spec:
  myField: |
    o problema é
    que eu tenho mania de 
    adicionar espaços aleatórios.
EOF
kubectl get myapi multiline1 -o yaml
....
spec:
  myField: "o problema é\nque eu tenho mania de \nadicionar espaços aleatórios.\n"
```

E essa situação acontece porque na 2a linha eu adicionei um espaço no final...se eu 
remover esse espaço tudo funciona:

```shell
kubectl apply -f - <<EOF
apiVersion: home.lab/v1
kind: MyAPI
metadata:
  name: multiline2
spec:
  myField: |
    o problema é
    que eu tenho mania de
    adicionar espaços aleatórios.
EOF
kubectl get myapi multiline2 -o yaml
....
spec:
  myField: |
    o problema é
    que eu tenho mania de
    adicionar espaços aleatórios.
```

Se você já precisou usar um helm chart pra instalar algo, e foi
buscar campos que tinham multiplas linhas deve ter visto algo do tipo já, e pensou "como
eu vou entender o que estava ai originalmente???"

Bom, vamos ver o output do nosso objeto `multiline1` usando kyaml:
```yaml
spec: {
    myField: "\
       o problema é\n\
       que eu tenho mania de \n\
       adicionar espaços aleatórios.\n\
      ",
  },
```

E veja só, o espaço está ali, mas ainda assim dá pra entender cada uma das linhas :) 

## Mas não tinha algo melhor? Vou precisar mudar algo no meu ambiente?
Bom, essa é uma pergunta que eu ouvi bastante. Antes de mais nada, deixa eu lembrar 
uma coisa: O Kubernetes é um projeto extremamente complexo, e qualquer nova adição exige
muita discussão, medir impactos, etc.

Isso dito, vamos dar uma olhada em como o Kubernetes usa esses YAML.

Todos os resources, etc são armazenados no [etcd](https://etcd.io). Se nós dermos uma 
olhada nos objetos que criamos acima, veremos algo muito interessante (a localizacão do 
etcdctl pode variar, aqui estou apenas dando um exemplo):

```shell
etcdctl get /registry/home.lab/myapis/default/multiline1  --print-value-only
```

Vai retornar o valor abaixo:
```json
{"apiVersion":"home.lab/v1","kind":"MyAPI","metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"home.lab/v1\",\"kind\":\"MyAPI\",\"metadata\":{\"annotations\":{},\"name\":\"multiline1\",\"namespace\":\"default\"},\"spec\":{\"myField\":\"o problema é\\nque eu tenho mania de \\nadicionar espaços aleatórios.\\n\"}}\n"},"creationTimestamp":"2025-08-29T14:12:03Z","generation":1,"managedFields":[{"apiVersion":"home.lab/v1","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:annotations":{".":{},"f:kubectl.kubernetes.io/last-applied-configuration":{}}},"f:spec":{".":{},"f:myField":{}}},"manager":"kubectl-client-side-apply","operation":"Update","time":"2025-08-29T14:12:03Z"}],"name":"multiline1","namespace":"default","uid":"32ec040d-4a95-4f9f-a187-bf0fb3ce9e69"},"spec":{"myField":"o problema é\nque eu tenho mania de \nadicionar espaços aleatórios.\n"}}
```

Mas veja só, nosso objeto é armazenado como um json! Na real não é bem assim, ele é um protobuffer 
que pode ser convertido pra Json, mas isso é outra história.

O fato aqui é: **seu YAML não é armazenado como YAML!**. Ele usa um outro formato 
sempre que pode, por sinal ser transformado em YAML. Quem faz essa função, no caso do `kubectl`
é uma biblioteca do Kubernetes chamada `cli-runtime` que possui os `printers` suportados.

Uma forma similar de verificar isso é chamando seu kubectl com o argumento "-v=9", veja:

```shell
kubectl get myapi multiline1 -v=9
....
"Response Body" body=<
	{"kind":"Table","apiVersion":"meta.k8s.io/v1","metadata":{"resourceVersion":"3697"},"columnDefinitions":[{"name":"Name","type":"string","format":"name","description":"Name must be unique within a namespace. Is required when creating resources, although some resources may allow a client to request the generation of an appropriate name automatically. Name is primarily intended for creation idempotence and configuration definition. Cannot be updated. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names#names","priority":0},{"name":"Age","type":"date","format":"","description":"CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC.\n\nPopulated by the system. Read-only. Null for lists. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata","priority":0}],"rows":[{"cells":["multiline1","35m"],"object":{"kind":"PartialObjectMetadata","apiVersion":"meta.k8s.io/v1","metadata":{"name":"multiline1","namespace":"default","uid":"32ec040d-4a95-4f9f-a187-bf0fb3ce9e69","resourceVersion":"3697","generation":1,"creationTimestamp":"2025-08-29T14:12:03Z","annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"home.lab/v1\",\"kind\":\"MyAPI\",\"metadata\":{\"annotations\":{},\"name\":\"multiline1\",\"namespace\":\"default\"},\"spec\":{\"myField\":\"o problema é\\nque eu tenho mania de \\nadicionar espaços aleatórios.\\n\"}}\n"},"managedFields":[{"manager":"kubectl-client-side-apply","operation":"Update","apiVersion":"home.lab/v1","time":"2025-08-29T14:12:03Z","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:annotations":{".":{},"f:kubectl.kubernetes.io/last-applied-configuration":{}}},"f:spec":{".":{},"f:myField":{}}}}]}}}]}
```

Pois é, seus dados estão ali armazenados e nem YAML eles são!

## Sobre o KYAML e sua compatibilidade com YAML
Nós já vimos que o Kubernetes armazena os dados sempre no mesmo formato no ETCD, que
os retornos da API são sempre JSON (e não yaml), e que toda essa conversão JSONxYAML 
acontece no lado do cliente.

Agora respondendo a `porque não um novo formato? Porque não CUE?`: 
Porque a idéia não era um novo formato, mas uma melhoria do YAML (que já é amplamente utilizado)
sem adicionar novas dependências no CLI, e que permita compatibilidade entre manifestsos existentes.

O uso do CUE ou de qualquer outro formato resolve a sua vontade de algo que você gosta mais (afinal, 
por que não usar FreeBSD então? Você usa emacs, eu gosto de VIM). Essas flamewars são antigas, 
e nunca ninguém vai estar 100% satisfeito. YAML significa `Yet Another Markup Language`, ou seja, 
apenas mais uma reinvenção de algo que já existe!

Isso dito, o Kubernetes foca muito na compatibilidade e em tentar ao máximo não quebrar o 
que já funciona.

O uso do KYAML não significa que um novo YAML foi criado, apenas que a forma como o 
Kubernetes usa o YAML foi melhorada.

Vamos dar uma olhada se é realmente compatível? Para os testes abaixo, você precisa do 
`kubectl` instalado, e a variável de ambiente `KUBECTL_KYAML=true` definida.

Primeiro, será que eu consigo usar o `yq` (uma ferramenta de linha de comando) pra mudar um dado 
de um recurso meu, usando KYAML?

```shell
kubectl get myapi not-a-bool -o kyaml |yq '.apiVersion = "xpto"'
---
{apiVersion: "xpto", kind: "MyAPI", metadata: {annotations: {kubectl.kubernetes.io/last-applied-configuration: "{\"apiVersion\":\"home.lab/v1\",\"kind\":\"MyAPI\",\"metadata\":{\"annotations\":{},\"name\":\"not-a-bool\",\"namespace\":\"default\"},\"spec\":{\"myField\":\"yes\"}}\n"}, creationTimestamp: "2025-08-29T13:58:30Z", generation: 1, name: "not-a-bool", namespace: "default", resourceVersion: "2710", uid: "2cde1013-8f97-4c54-bda1-73b6f0d48b95"}, spec: {myField: "yes"}}
```

Interessante, então o KYAML funciona para o `yq`. Pois é, veja algo mais legal:
```shell
kubectl get myapi not-a-bool -o kyaml |yq '.spec.myField = "No"' |kubectl apply -f -
kubectl get myapi not-a-bool -o yaml
...
spec:
  myField: "No"
```

Eu baixei um recurso usando o KYAML, mudei o campo, apliquei de novo, e quando dei um 
`kubectl get` usando `-o yaml` vi que o campo mudou. Sabe quem ficaria muito feliz 
com esse novo formato? Aquele seu FluxCD/ArgoCD que fica reaplicando coisas e não 
se importa se o manifesto é KYAML ou não, e você quando precisar dar um get num 
objeto que tenha um campo string com multiplas linhas :P 

Fica a dica: **KYAML ainda é um YAML, ele só é mais estrito em como os manifestos serão apresentados"

Isso fica evidente inclusive com outras linguagens como Python, vamos ver?

```shell
kubectl get myapi not-a-bool -o kyaml | python -c 'import sys, yaml; print(yaml.safe_load(sys.stdin)["spec"]["myField"])'
No
```

## Conclusões

Vou concluir respondendo às perguntas originais de novo:

* Puxa, mais um formato diferente? Não era melhor ter usado CUE? YAML já era ruim, 
JSON com YAML então, o que os mantenedores tem na cabeça?

Respondendo essas 3 de uma vez, larga de ser chato, que quando seu cluster Kubernetes quebra 
você vai reclamar que os mantenedores do Kubernetes não pensam no seu caso :P 

Mas falando sério, conforme explicado o KYAML é só um YAML, mas ele é mais verboso, 
imprime caracteres que são geralmente ocultos, tornando o manifesto mais explícito 
e menos ambíguo.

* Eu estou curioso pra saber o que justificou isso.
Acho que eu expliquei tudo, mas o [Kubernetes Enhancement Proposal](https://github.com/kubernetes/enhancements/tree/master/keps/sig-cli/5295-kyaml) 
também tem boas explicações.

* Vai quebrar um monte de coisa
Não vai, você usa o KYAML se quiser. Eu sugiro usar nos seus pipelines, quando estiver 
escrevendo manifestos complexos, etc. O KYAML é um YAML, ele é compatível com YAML, 
etc (não vou me repetir mais!)
