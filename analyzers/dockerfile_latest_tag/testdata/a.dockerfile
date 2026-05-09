FROM ubuntu:latest        # want "pin a specific image tag"
FROM ubuntu               # want "image has no tag"
FROM golang:1.21
FROM alpine AS builder    # want "image has no tag"
FROM debian:12-slim
