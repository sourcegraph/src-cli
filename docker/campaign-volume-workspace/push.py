#!/usr/bin/env python3

import argparse
import itertools
import os
import subprocess

from typing import BinaryIO, Optional, Sequence


def calculate_tags(ref: str) -> Sequence[str]:
    # The tags always include latest.
    tags = ["latest"]

    # If the ref is a tag ref, then we should parse the version out.
    if ref.startswith("refs/tags/"):
        tags.extend(
            [
                ".".join(vc)
                for vc in itertools.accumulate(
                    ref.split("/", 2)[2].split("."),
                    lambda vlist, v: vlist + [v],
                    initial=[],
                )
                if len(vc) > 0
            ]
        )

    return tags


def docker_build(
    dockerfile: BinaryIO, platform: Optional[str], image: str, tags: Sequence[str]
):
    args = ["docker", "buildx", "build", "--push"]

    for tag in tags:
        args.extend(["-t", tag])

    if platform is not None:
        args.extend(["--platform", platform])

    args.append("-")

    run(args, stdin=dockerfile)


def docker_login(username: str, password: str):
    run(
        ["docker", "login", f"-u={username}", "--password-stdin"],
        input=password,
        text=True,
    )


def run(args: Sequence[str], /, **kwargs) -> subprocess.CompletedProcess:
    print(f"+ {' '.join(args)}")
    return subprocess.run(args, check=True, **kwargs)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "-d", "--dockerfile", required=True, help="the Dockerfile to build"
    )
    parser.add_argument("-i", "--image", required=True, help="Docker image to push")
    parser.add_argument(
        "-p",
        "--platform",
        help="platforms to build using docker buildx (if omitted, the default will be used)",
    )
    parser.add_argument(
        "-r",
        "--ref",
        default=os.environ.get("GITHUB_REF"),
        help="current ref in refs/heads/... or refs/tags/... form",
    )
    args = parser.parse_args()

    tags = calculate_tags(args.ref)
    print(f"will push tags: {', '.join(tags)}")

    print("logging into Docker Hub")
    try:
        docker_login(os.environ["DOCKER_USERNAME"], os.environ["DOCKER_PASSWORD"])
    except KeyError as e:
        print(f"error retrieving environment variables: {e}")
        raise

    print("building and pushing image")
    docker_build(open(args.dockerfile, "rb"), args.platform, args.image, tags)

    print("success!")


if __name__ == "__main__":
    main()
