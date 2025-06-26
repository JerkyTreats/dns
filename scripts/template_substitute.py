#!/usr/bin/env python3
"""
Safe template substitution script for config files.
This replaces the fragile sed-based approach with robust Python string replacement.
"""

import sys
import os
import argparse


def substitute_template(template_file, output_file, substitutions):
    """
    Safely substitute placeholders in a template file.

    Args:
        template_file: Path to the template file
        output_file: Path to write the output file
        substitutions: Dict of placeholder -> replacement mappings
    """
    try:
        # Read the template file
        with open(template_file, 'r', encoding='utf-8') as f:
            content = f.read()

        # Perform substitutions
        for placeholder, replacement in substitutions.items():
            content = content.replace(placeholder, replacement)

        # Write the output file
        with open(output_file, 'w', encoding='utf-8') as f:
            f.write(content)

        print(f"Template substitution completed: {template_file} -> {output_file}")

    except Exception as e:
        print(f"Error during template substitution: {e}", file=sys.stderr)
        sys.exit(1)


def main():
    parser = argparse.ArgumentParser(description='Safe template substitution')
    parser.add_argument('template_file', help='Template file path')
    parser.add_argument('output_file', help='Output file path')
    parser.add_argument('--substitute', '-s', action='append', nargs=2,
                       metavar=('PLACEHOLDER', 'REPLACEMENT'),
                       help='Placeholder and replacement pair (can be used multiple times)')

    args = parser.parse_args()

    if not args.substitute:
        print("Error: No substitutions provided. Use --substitute PLACEHOLDER REPLACEMENT", file=sys.stderr)
        sys.exit(1)

    # Build substitutions dictionary
    substitutions = {}
    for placeholder, replacement in args.substitute:
        substitutions[placeholder] = replacement

    substitute_template(args.template_file, args.output_file, substitutions)


if __name__ == '__main__':
    main()
