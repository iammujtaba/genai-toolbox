---
title: "redis"
type: docs
weight: 1
description: > 
  A "redis" tool executes a set of pre-defined Redis commands against a Memorystore for Redis instance.

---

## About

A redis tool executes a series of pre-defined Redis commands against a
Memorystore for Redis instance. It's compatible with the following source:

- memorystore-redis

The specified Redis commands are executed sequentially. Each command is
represented as a string list, where the first element is the command name (e.g., SET,
GET, HGETALL) and subsequent elements are its arguments. Dynamic command
arguments can be templated using [Go template][go-template-doc]'s annotations.

[go-template-doc]: <https://pkg.go.dev/text/template#pkg-overview>

## Example

```yaml
tools:
  user_data_tool:
    kind: redis
    source: my-redis-instance
    description: |
      Use this tool to interact with user data stored in Redis.
      It can set, retrieve, and delete user-specific information.
    commands:
      - [SET, user:{{.userId}}:name, {{.userName}}]
      - [GET, user:{{.userId}}:email]
    parameters:
      - name: userId
        type: string
        description: The unique identifier for the user.
      - name: userName
        type: string
        description: The name of the user to set.
```
