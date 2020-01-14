FROM alpine:3
ADD ./github-action-workflow-golang.yml /tmp/workflows/

CMD mkdir -p .github/workflows && \
	DEST=.github/workflows/lsif.yml; \
	if [ ! -f .github/workflows/lsif.yml ]; then \
		cp /tmp/workflows/github-action-workflow-golang.yml $DEST; \
	else \
		echo Doing nothing because existing LSIF workflow found at $DEST; \
	fi
