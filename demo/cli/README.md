# Platform Service's Lootbox Roll Plugin gRPC Demo App (Go)

## Overview

This is CLI program to test the usage of custom plugin lootbox roll function configuration on AccelByte environment

> Please note that this script will delete and recreate your e-commerce store draft in your specified namespace 

## Prerequisites

- Docker
- make

## Setup

The following environment variables are used by this CLI demo app.

Put environment variables in .env file:

```shell
AB_BASE_URL='https://test.accelbyte.io'
AB_CLIENT_ID='<AccelByte IAM Client ID>'
AB_CLIENT_SECRET='<AccelByte IAM Client Secret>'

AB_NAMESPACE='namespace'
AB_USERNAME='<AccelByte account username>'
AB_PASSWORD='<AccelByte account password>'
```

## Run 

Run the demo test cli using makefile with provided `.env` file containing required variables

```shell
make run ENV_FILE_PATH=.env
```