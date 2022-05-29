#!/usr/bin/env bash


message_newline() {
    echo
}

message_debug()
{
    echo -e "DEBUG: ${@}"
}

message_welcome()
{
    echo -e "\e[1m${@}\e[0m"
}

message_warning()
{
    echo -e "\e[33mWARNING\e[0m: ${@}"
}

message_error()
{
    echo -e "\e[31mERROR\e[0m: ${@}"
}

message_info()
{
    echo -e "\e[37mINFO\e[0m: ${@}"
}

message_suggestion()
{
    echo -e "\e[33mSUGGESTION\e[0m: ${@}"
}

message_success()
{
    echo -e "\e[32mSUCCESS\e[0m: ${@}"
}
