---
layout: default
title: Blog
has_children: true
nav_order: 3
parent: ''
---

# g8e Blog

Last Updated: 2026-05-07
Version: v0.2.0

Welcome to the g8e blog.

## Posts

{% for page in site.pages %}
  {% if page.parent == "Blog" and page.title != "Blog" %}
  - [{{ page.title }}]({{ page.url | relative_url }})
  {% endif %}
{% endfor %}

