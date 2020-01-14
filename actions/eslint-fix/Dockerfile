FROM node:12-alpine3.10
VOLUME /cache
ENV YARN_CACHE_FOLDER=/cache/yarn
ENV npm_config_cache=/cache/npm
CMD package_json_bkup=$(mktemp) && \
	cp package.json $package_json_bkup && \
	yarn -s --non-interactive --pure-lockfile --ignore-optional --ignore-scripts --ignore-engines --ignore-platform --no-progress && \
	yarn add --ignore-workspace-root-test --non-interactive --ignore-optional --ignore-scripts --ignore-engines --ignore-platform --pure-lockfile -D @typescript-eslint/parser @typescript-eslint/eslint-plugin --no-progress eslint && \
	node_modules/.bin/eslint \
	--fix \
	--plugin @typescript-eslint \
	--parser @typescript-eslint/parser \
	--parser-options '{"ecmaVersion": 8, "sourceType": "module", "project": "tsconfig.json"}' \
	--rule '@typescript-eslint/prefer-optional-chain: 2' \
	--rule '@typescript-eslint/no-unnecessary-type-assertion: 2' \
	--rule '@typescript-eslint/no-unnecessary-type-arguments: 2' \
	--rule '@typescript-eslint/no-unnecessary-condition: 2' \
	--rule '@typescript-eslint/no-unnecessary-type-arguments: 2' \
	--rule '@typescript-eslint/prefer-includes: 2' \
	--rule '@typescript-eslint/prefer-readonly: 2' \
	--rule '@typescript-eslint/prefer-string-starts-ends-with: 2' \
	--rule '@typescript-eslint/prefer-nullish-coalescing: 2' \
	--rule '@typescript-eslint/no-non-null-assertion: 2' \
	'**/*.ts'; \
	mv $package_json_bkup package.json; \
	rm -rf node_modules; \
	yarn upgrade --latest --ignore-workspace-root-test --non-interactive --ignore-optional --ignore-scripts --ignore-engines --ignore-platform --no-progress typescript && \
	rm -rf node_modules