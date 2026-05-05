---
layout: default
title: Blog
has_children: true
nav_order: 3
parent: ''
---

# g8e Blog

Welcome to the g8e blog.

## Posts

{% for page in site.pages %}
  {% if page.parent == "Blog" and page.title != "Blog" %}
  - [{{ page.title }}]({{ page.url | relative_url }})
  {% endif %}
{% endfor %}

