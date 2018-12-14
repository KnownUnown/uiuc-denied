#!/usr/bin/env sh

eval $(cat .credentials)
go run main.go -pushkey $PUSHKEY -recipient $RECIPIENT -username $USERNAME -password $PASSWORD
