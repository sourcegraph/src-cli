const fs = require('fs')
const path = require('path')

const licensePath = path.join(__dirname, '..', 'LICENSE')
const readmePath = path.join(__dirname, '..', 'README.md')

fs.copyFileSync(licensePath, path.join(__dirname, 'LICENSE'))
fs.copyFileSync(readmePath, path.join(__dirname, 'README.md'))
