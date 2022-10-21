# Contributing Guide

Please see our [general guide for OSS lifecycle and practices.](https://github.com/honeycombio/home/blob/main/honeycomb-oss-lifecycle-and-practices.md)

## Design

### localMode

```mermaid
sequenceDiagram
  main->>eventpublisher: New
  eventpublisher->>main: Here you go
  main->>logsapi: Start logs receiver with eventpublisher

  Note left of logsapi: JSON payload to localhost:3000 of stdout log entries
  loop
    logsapi->>eventpublisher: server: transform log entries and enqueue events
  end
  loop
    eventpublisher->>eventpublisher: Batch timeout
    eventpublisher->>HoneycombAPI: Ship events
  end
  Note right of main: Receive signal to TERM/QUIT
  main->>main: Cancel Context
  main->>main: Exit
```

### In Lambda

```mermaid
sequenceDiagram
  main->>eventpublisher: New
  eventpublisher->>main: Here you go
  main->>logsapi: Start logs receiver with eventpublisher

  main->>extensionclient: Start
  extensionclient->>AWSExtensionAPI: Register


  main->>logsapi: Subscribe
  logsapi->>AWSExtensionAPI: client: subscribe to lambda event types
  main->>eventprocessor: Start

  par Lambda sends stdout entries to Extension's log receiver

    loop
      AWSExtensionAPI->>logsapi: Publish lambda function stdout
      logsapi->>eventpublisher: server: transform log entry and enqueue events event
    end

  and eventpublisher ships event batches to Honeycomb on a time interval
    loop
      eventpublisher->>eventpublisher: Batch timeout
      Note right of eventpublisher: Lambda might sleep, so might continue during next Invoke
      eventpublisher->>HoneycombAPI: Ship events
    end

  and eventprocessor handles Lambda events (Invoke, Shutdown)

    loop
      eventprocessor->>extensionclient: Poll for Lambda Events
      extensionclient->>AWSExtensionAPI: Get Lambda Events
      AWSExtensionAPI->>extensionclient: Return Lambda Events
      extensionclient->>eventprocessor: Handle Lambda Events
    end

  end

  AWSExtensionAPI->>eventprocessor: Lambda Shutdown
  eventprocessor->>eventpublisher: Enqueue shutdown event
  eventprocessor->>eventpublisher: Flush event queue
  eventpublisher->>HoneycombAPI: Ship events
  eventprocessor->>eventprocessor: Cancel Context
  eventprocessor->>main: Return from execution
  main->>main: Exit
```
