#!/usr/bin/env python3
"""Generate embeddings for documentation RAG search.

This script processes the built MkDocs site and generates embeddings
for all markdown content, storing them in a JSON vector map for
client-side similarity search.
"""

from __future__ import annotations

import json
import os
import re
from pathlib import Path
from typing import Any

try:
    from sentence_transformers import SentenceTransformer
except ImportError:
    print("Error: sentence-transformers not installed.")
    print("Install with: pip install sentence-transformers")
    exit(1)


def clean_text(text: str) -> str:
    """Clean markdown text for embedding generation."""
    # Remove code blocks
    text = re.sub(r'```.*?```', '', text, flags=re.DOTALL)
    # Remove inline code
    text = re.sub(r'`[^`]+`', '', text)
    # Remove links but keep text
    text = re.sub(r'\[([^\]]+)\]\([^)]+\)', r'\1', text)
    # Remove headers
    text = re.sub(r'^#+\s+', '', text, flags=re.MULTILINE)
    # Normalize whitespace
    text = re.sub(r'\s+', ' ', text).strip()
    return text


def extract_content_from_html(html_path: Path) -> dict[str, Any]:
    """Extract text content from built HTML file."""
    try:
        with open(html_path, 'r', encoding='utf-8') as f:
            html = f.read()
        
        # Extract title from h1
        title_match = re.search(r'<h1[^>]*>(.*?)</h1>', html, re.DOTALL)
        title = title_match.group(1) if title_match else html_path.stem
        
        # Extract main content from article tag
        content_match = re.search(r'<article[^>]*>(.*?)</article>', html, re.DOTALL)
        if content_match:
            content = content_match.group(1)
        else:
            # Fallback to body
            content_match = re.search(r'<body[^>]*>(.*?)</body>', html, re.DOTALL)
            content = content_match.group(1) if content_match else ''
        
        # Clean HTML tags
        text = re.sub(r'<[^>]+>', ' ', content)
        text = clean_text(text)
        
        # Get relative URL
        relative_path = html_path.relative_to(html_path.parent.parent).as_posix()
        url = relative_path.replace('index.html', '').replace('.html', '')
        if url.endswith('/'):
            url = url[:-1]
        
        return {
            'url': url or '/',
            'title': clean_text(title),
            'content': text,
        }
    except Exception as e:
        print(f"Warning: Failed to process {html_path}: {e}")
        return None


def generate_embeddings(
    site_dir: Path,
    output_file: Path,
    model_name: str = 'all-MiniLM-L6-v2',
    chunk_size: int = 500,
    chunk_overlap: int = 50,
) -> None:
    """Generate embeddings for all documentation pages."""
    print(f"Loading model: {model_name}")
    model = SentenceTransformer(model_name)
    
    # Find all HTML files
    html_files = list(site_dir.rglob('*.html'))
    print(f"Found {len(html_files)} HTML files")
    
    # Extract content
    documents = []
    for html_file in html_files:
        if '404' in html_file.name or 'search' in html_file.name:
            continue
        
        doc = extract_content_from_html(html_file)
        if doc and doc['content']:
            documents.append(doc)
    
    print(f"Extracted content from {len(documents)} pages")
    
    # Chunk documents
    chunks = []
    for doc in documents:
        text = doc['content']
        words = text.split()
        
        for i in range(0, len(words), chunk_size - chunk_overlap):
            chunk_text = ' '.join(words[i:i + chunk_size])
            if len(chunk_text) > 50:  # Skip very short chunks
                chunks.append({
                    'url': doc['url'],
                    'title': doc['title'],
                    'content': chunk_text,
                })
    
    print(f"Created {len(chunks)} text chunks")
    
    # Generate embeddings
    print("Generating embeddings...")
    texts = [chunk['content'] for chunk in chunks]
    embeddings = model.encode(texts, show_progress_bar=True)
    
    # Prepare output
    output_data = {
        'model': model_name,
        'chunks': chunks,
        'embeddings': embeddings.tolist(),
    }
    
    # Write output
    output_file.parent.mkdir(parents=True, exist_ok=True)
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(output_data, f)
    
    print(f"Embeddings saved to {output_file}")
    print(f"Total size: {output_file.stat().st_size / 1024 / 1024:.2f} MB")


def main() -> None:
    """Main entry point."""
    import argparse
    
    parser = argparse.ArgumentParser(description='Generate embeddings for documentation RAG')
    parser.add_argument(
        '--site-dir',
        type=Path,
        default=Path('site'),
        help='Path to built MkDocs site directory'
    )
    parser.add_argument(
        '--output',
        type=Path,
        default=Path('docs/embeddings.json'),
        help='Output path for embeddings JSON file'
    )
    parser.add_argument(
        '--model',
        type=str,
        default='all-MiniLM-L6-v2',
        help='Sentence transformer model name'
    )
    
    args = parser.parse_args()
    
    if not args.site_dir.exists():
        print(f"Error: Site directory {args.site_dir} does not exist")
        print("Run 'mkdocs build' first")
        exit(1)
    
    generate_embeddings(args.site_dir, args.output, args.model)


if __name__ == '__main__':
    main()
