---
title: "Experimenting mcp-go, AnythingLLM and local LLM executions"
date: 2025-04-21T16:16:00-03:00
draft: false
description: ""
featured: true # Sets if post is a featured post, making appear on the home page side bar.
toc: true 
codeMaxLines: 100 # Override global value for how many lines within a code block before auto-collapsing.
codeLineNumbers: true # Override global value for showing of line numbers within code block.
figurePositionShow: true # Override global value for showing the figure label.
categories:
  - Technology
tags:
  - MCP
  - LLM

---

# Experimenting with MCP-Go, AnythingLLM and local executions

**NOTE**: I am no LLM, AI or whatever expert. Far from this. I have decided to write
this blog post mostly as a way to track my progress and share my findings! There may be some 
mistakes here, please let me know!

I got hyped into the [MCP](https://modelcontextprotocol.io/) trend. Usually I wait
a bit more to get engaged into these subjects, but when it comes to cool integration
between tools, I get excited to experiment fast.

MCP, as a single line explanation (from my own head) is "a way to integrate local data
with LLM". Mostly it allows me to connect some LLM that understands a question like 
"what's the current weather in Sao Paulo", providing an interface that receives a formatted
query, requests informations from some API (but can also be a full local execution, like
listing my filesystem) and provides a structured response to LLM back so it can be formatted.

It is, as its name says, a protocol that allows a developer to create an "API'ish", 
queried in real time and providing answers based on the API response.

I have decided to use [Golang SDK](https://github.com/mark3labs/mcp-go/) as it is my comfort zone, 
but the MCP site has a bunch of useful examples using whatever language you feel more comfortable
with (and it also has its official SDKs in Python, NodeJS, etc).

Because I don't have enough money to spend calling Claude (Anthropic, the original creator
of MCP is also the company behind Claude), I have decided to use the very cool project
called [AnythingLLM](https://anythingllm.com/) that allows me to execute local models
and also supports MCP.

# Concepts
Some concepts used here:
* MCP Server - AKA Agent AI - What we are implementing here. A "server" that will be called to provide responses
* MCP Client - A client that "knows" how to call a MCP server. In our case is AnythingLLM
* Resources - Our API responses, but can be local files, etc. Consumed by MCP Server, provided as response to MCP Client
* Tools - the "methods" that can be called by MCP Client. In our case you are going to see it as the `getForecast()` function


# Pre requisites
I will not cover how to install every piece, it should be straightforward. What you need
is to install [AnythingLLM](https://anythingllm.com/) and load a model. I am using Llama 3.2 3B, 
but if you need more complex operations, AnythingLLM allows you to select different models
to execute locally.

The API used in this example is based on already existing example 
[Weather MCP](https://github.com/modelcontextprotocol/quickstart-resources/tree/main/weather-server-python)
but mostly converted to Go. We cover just the forecast API, and not the Alerts one, but go
ahead and try implementing your own :) it is fun!

The API definition is also available [here](https://www.weather.gov/documentation/services-web-api)

# Getting started
First, we need to create a Go project with MCP Go library:

```shell
go mod init weather-go
go get github.com/mark3labs/mcp-go
```

## Defining the API responses
The forecast API makes its responses as JSON. To make our life easier, only the parts
we care from this API were extracted, so the definition looks like:

```golang
// WeatherPoints contains the response for API/points/latitude,longitude
// we care just about the Forecast field, that contains the new API URL that should be called
type WeatherPoints struct {
	Properties struct {
		Forecast string `json:"forecast"`
	} `json:"properties"`
}

// Forecast contains the forecast prediction for a lat,long. 
// example: https://api.weather.gov/gridpoints/MTR/85,105/forecast
type Forecast struct {
	Properties struct {
		Periods []struct {
			Name             string `json:"name,omitempty"`
			Temperature      int    `json:"temperature,omitempty"`
			TemperatureUnit  string `json:"temperatureUnit,omitempty"`
			WindSpeed        string `json:"windSpeed,omitempty"`
			WindDirection    string `json:"windDirection,omitempty"`
			ShortForecast    string `json:"shortForecast,omitempty"`
			DetailedForecast string `json:"detailedForecast,omitempty"`
		} `json:"periods,omitempty"`
	} `json:"properties,omitempty"`
}
```

## Building the Forecast tool
The Forecast tool is how we expose to our client how/when the Forecast should be called.
It requests for parameters like "a City name", "a file location", "what numbers should be suummed".

The snippets below define the tool and the call execution. Later we will add it to the
main MCP server.

Let's take a look into the function:

```golang
import (
    "github.com/mark3labs/mcp-go/mcp"
)
// Add forecast tool
func getForecast() mcp.Tool {
	weatherTool := mcp.NewTool("get_forecast",
		mcp.WithDescription("Get weather forecast for a location"),
		mcp.WithNumber("latitude",
			mcp.Required(),
			mcp.Description("Latitude of the location"),
		),
		mcp.WithNumber("longitude",
			mcp.Required(),
			mcp.Description("Longitude of the location"),
		),
	)
	return weatherTool
}
```

When the MCP Client connects to a MCP Server, the MCP server exposes get_forecast as a 
valid tool, requesting latitute and longitude parameters. 

**Note**: I don't understand yet the magics between a prompt receiving a request like "give me 
the forecast of bla" and deferring to the get_forecast tool! 

Setting the forecast tool is not enough. We need to define to our server, once a "get_forecast" call
is made, what it should execute. First, we define a util function to call the API:

```golang
const (
    nws_api_base       = "https://api.weather.gov"
	user_agent         = "weather-app/1.0"
)
func makeNWSRequest(requrl string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, requrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", user_agent)
	req.Header.Set("Accept", "application/geo+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}
```

Then we need the proper function that will make requests to the API, and format 
the response properly:

```golang
const (
    // We just want the next 5 forecast results
    maxForecastPeriods = 5
)
func forecastCall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // Fetches latitude and longitude passed to the tool call
	latitude := request.Params.Arguments["latitude"].(float64)
	longitude := request.Params.Arguments["longitude"].(float64)

    // Makes the API call.
	pointsURL := fmt.Sprintf("%s/points/%.4f,%.4f", nws_api_base, latitude, longitude)
	points, err := makeNWSRequest(pointsURL)
	if err != nil {
		return nil, fmt.Errorf("error calling points API: %w", err)
	}
	pointsData := &WeatherPoints{}
	if err := json.Unmarshal(points, pointsData); err != nil {
		return nil, fmt.Errorf("error unmarshalling points data: %w", err)
	}

	if pointsData == nil || pointsData.Properties.Forecast == "" {
		return nil, fmt.Errorf("points does not contain forecast url")
	}

    // Make a second API call with the Forecast URL to get the forecast result
	forecastReq, err := makeNWSRequest(pointsData.Properties.Forecast)
	if err != nil {
		return nil, fmt.Errorf("error getting the forecast data")
	}

	forecastData := &Forecast{}
	if err := json.Unmarshal(forecastReq, forecastData); err != nil {
		return nil, fmt.Errorf("error unmarshalling points data: %w", err)
	}

	type ForecastResult struct {
		Name        string `json:"Name"`
		Temperature string `json:"Temperature"`
		Wind        string `json:"Wind"`
		Forecast    string `json:"Forecast"`
	}

    // format a json containing the 5 forecasts
	forecast := make([]ForecastResult, maxForecastPeriods)
	for i, period := range forecastData.Properties.Periods {
		forecast[i] = ForecastResult{
			Name:        period.Name,
			Temperature: fmt.Sprintf("%d°%s", period.Temperature, period.TemperatureUnit),
			Wind:        fmt.Sprintf("%s %s", period.WindSpeed, period.WindDirection),
			Forecast:    period.DetailedForecast,
		}
        if i >= maxForecastPeriods-1 {
			break
		}
	}

	forecastResponse, err := json.Marshal(&forecast)
	if err != nil {
		return nil, fmt.Errorf("error marshalling forecast: %w", err)
	}

    // And return it as string back to the caller function
	return mcp.NewToolResultText(string(forecastResponse)), nil
}
```

## Defining the main server
With everything in place, we can define the main server, add our tool and the "handler" function
for that tool:

```golang
import (
    "github.com/mark3labs/mcp-go/server"
)
func main() {
	// Create a new MCP server
	s := server.NewMCPServer(
		"Weather Demo",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
		server.WithRecovery(),
	)

    // Add the forecast tool and its handler function
	s.AddTool(getForecast(), forecastCall)

	// Start the server
	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
```

With that in place, we can try simply building and executing our Go program. It should give us any 
return, and wait for inputs on STDIO. After that, interrupt it with Ctrl-C
```shell
go build -o forecast .
./forecast
```

# Integrating with AnythingLLM
The integration with AnythingLLM is made defining the MCP server binary that we want
to call. It is a JSON config, and its schema follows the same definition for Claude Desktop.

The configuration location may vary from each OS, on MacOS you define it on `~/Library/Application\ Support/anythingllm-desktop/storage/plugins/anythingllm_mcp_servers.json`.

Following is how mine is defined:
```json
{
    "mcpServers": {
        "weather": {
            "command": "/Users/rkatz/codes/mcp/weather-go/forecast"
        }
    }
}
```

Once it is defined, we can verify if it is properly working on the UI:
![AnythingLLM Agent Configuration](/images/mcpgo/agent.png)

As we can see it is working, we can make a prompt call like:
```raw
@agent what's the forecast for Atlanta?
```

It is important to use the `@agent` on the beginning of the Prompt. AnythingLLM requires
this to understand this should be sent to MCP Server.

![AnythingLLM Prompt](/images/mcpgo/prompt.png)

With that, we can see that AnythingLLM deferred the call to MCP Server, made the call
and formatted the output.

# Debugging
Sometimes things may go wrong. The most common mistake I have faced is **Wrong command call** on AnythingLLM configuration.

If the prompt answers there's some error on the Agent/MCP Server, you can request for the prompt
for some more information, like `Can you print the full error` or `can you print the Json response from Server`
and it should print some useful information.


# Acknowledges
My acknowledge here goes to my friend Anderson Duboc, that helped me start scratching
this surface with all the patience that I require! :) 

Also, for the folks who created this amazing SDK and saved me a lot of time.