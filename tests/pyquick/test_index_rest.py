#!/usr/bin/env python
# coding: utf-8
#
#

import requests

HOST = 'http://localhost:3000'

def test_index_get():
    r = requests.get(HOST + '/')
    assert r.status_code == 200

