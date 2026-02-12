#!/usr/bin/env python3
"""Parse benchmark results and generate comparison summary."""

import csv
import json
import os
import sys
from pathlib import Path

def parse_k6_summary(filepath):
    """Parse k6 JSON summary output."""
    with open(filepath) as f:
        data = json.load(f)

    metrics = data.get('metrics', {})
    result = {}

    # HTTP request duration (latency)
    if 'http_req_duration' in metrics:
        d = metrics['http_req_duration']['values']
        result['latency'] = {
            'avg': round(d.get('avg', 0), 2),
            'p50': round(d.get('med', 0), 2),
            'p90': round(d.get('p(90)', 0), 2),
            'p95': round(d.get('p(95)', 0), 2),
            'p99': round(d.get('p(99)', 0), 2),
            'min': round(d.get('min', 0), 2),
            'max': round(d.get('max', 0), 2),
        }

    # TTFB
    if 'http_req_waiting' in metrics:
        d = metrics['http_req_waiting']['values']
        result['ttfb'] = {
            'avg': round(d.get('avg', 0), 2),
            'p50': round(d.get('med', 0), 2),
            'p95': round(d.get('p(95)', 0), 2),
            'p99': round(d.get('p(99)', 0), 2),
        }

    # Request rate
    if 'http_reqs' in metrics:
        result['rps'] = round(metrics['http_reqs']['values'].get('rate', 0), 2)
        result['total_requests'] = metrics['http_reqs']['values'].get('count', 0)

    # Custom metrics
    if 'success_rate' in metrics:
        result['success_rate'] = round(metrics['success_rate']['values'].get('rate', 0) * 100, 2)

    if 'error_count' in metrics:
        result['error_count'] = metrics['error_count']['values'].get('count', 0)

    return result


def parse_resources(filepath):
    """Parse resource monitoring CSV."""
    rows = []
    with open(filepath) as f:
        reader = csv.DictReader(f)
        for row in reader:
            rows.append({
                'timestamp': int(row['timestamp']),
                'cpu': float(row['cpu_percent']),
                'rss_mb': float(row['rss_mb']),
                'fd_count': int(row['fd_count']),
                'threads': int(row['threads']),
            })

    if not rows:
        return {}

    cpus = [r['cpu'] for r in rows]
    rss = [r['rss_mb'] for r in rows]
    fds = [r['fd_count'] for r in rows]

    return {
        'cpu': {
            'avg': round(sum(cpus) / len(cpus), 2),
            'max': round(max(cpus), 2),
            'min': round(min(cpus), 2),
        },
        'memory_mb': {
            'avg': round(sum(rss) / len(rss), 2),
            'max': round(max(rss), 2),
            'min': round(min(rss), 2),
            'growth': round(rss[-1] - rss[0], 2),
        },
        'fd_count': {
            'avg': round(sum(fds) / len(fds)),
            'max': max(fds),
        },
        'samples': len(rows),
    }


def main():
    if len(sys.argv) < 2:
        print("Usage: python parse.py <results_dir>")
        print("  results_dir should contain go/, node/, next/ subdirectories")
        sys.exit(1)

    results_dir = Path(sys.argv[1])
    gateways = ['go', 'node', 'next']
    scenarios = [f's{i:02d}' for i in range(1, 11)]

    summary = {}

    for gw in gateways:
        gw_dir = results_dir / gw
        if not gw_dir.exists():
            continue

        summary[gw] = {}

        for scenario in scenarios:
            # Look for k6 summary
            k6_file = gw_dir / f'{scenario}-summary.json'
            res_file = gw_dir / f'{scenario}-resources.csv'

            entry = {}
            if k6_file.exists():
                entry['performance'] = parse_k6_summary(k6_file)
            if res_file.exists():
                entry['resources'] = parse_resources(res_file)

            if entry:
                summary[gw][scenario] = entry

    # Print comparison table
    print("\n" + "=" * 100)
    print("LLM GATEWAY BENCHMARK RESULTS")
    print("=" * 100)

    for scenario in scenarios:
        has_data = any(scenario in summary.get(gw, {}) for gw in gateways)
        if not has_data:
            continue

        print(f"\n--- {scenario.upper()} ---")
        print(f"{'Metric':<25} {'Go':>15} {'Node.js':>15} {'Next.js':>15}")
        print("-" * 70)

        for metric_name, getter in [
            ('RPS', lambda d: d.get('performance', {}).get('rps', '-')),
            ('Avg Latency (ms)', lambda d: d.get('performance', {}).get('latency', {}).get('avg', '-')),
            ('P50 Latency (ms)', lambda d: d.get('performance', {}).get('latency', {}).get('p50', '-')),
            ('P95 Latency (ms)', lambda d: d.get('performance', {}).get('latency', {}).get('p95', '-')),
            ('P99 Latency (ms)', lambda d: d.get('performance', {}).get('latency', {}).get('p99', '-')),
            ('Success Rate (%)', lambda d: d.get('performance', {}).get('success_rate', '-')),
            ('Avg CPU (%)', lambda d: d.get('resources', {}).get('cpu', {}).get('avg', '-')),
            ('Max CPU (%)', lambda d: d.get('resources', {}).get('cpu', {}).get('max', '-')),
            ('Avg Mem (MB)', lambda d: d.get('resources', {}).get('memory_mb', {}).get('avg', '-')),
            ('Max Mem (MB)', lambda d: d.get('resources', {}).get('memory_mb', {}).get('max', '-')),
            ('Mem Growth (MB)', lambda d: d.get('resources', {}).get('memory_mb', {}).get('growth', '-')),
            ('Max FD Count', lambda d: d.get('resources', {}).get('fd_count', {}).get('max', '-')),
        ]:
            vals = []
            for gw in gateways:
                data = summary.get(gw, {}).get(scenario, {})
                vals.append(str(getter(data)))
            print(f"{metric_name:<25} {vals[0]:>15} {vals[1]:>15} {vals[2]:>15}")

    # Save full summary as JSON
    output_file = results_dir / 'summary.json'
    with open(output_file, 'w') as f:
        json.dump(summary, f, indent=2)
    print(f"\nFull summary saved to: {output_file}")


if __name__ == '__main__':
    main()
