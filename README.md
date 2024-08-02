# Simple composer `packages.json` generator
This is a very small web services which can be used to create a composer `packages.json` from files stored on the file system. KISS! Additionally there exists an API endpoint which allows to upload composer packages, which are then stored according to the content of the `composer.json`.


## Why
I want to host private packages but no solution I found suited my needs. I need a very simple service where I can sent pre-generated ZIP files, containing a `composer.json` from a CI pipeline. For example the Gitlab composer package repository requires all files to be present in a git repository, since for a release I need to include generated data in the ZIP file and I do not want to keep generated data in the git repository this was not a viable solution for me. Also all other existing solutions I found were much too complex for my needs, and thus required too much configuration and maintenance. Additionally I do not need to distribute packages generated from some VCS.

Feel free to use this software for any other purpose, but note that I want to keep the API very lightweight and thus new features will only be added if I can integrate them into my use case. So feel free to open an issue to discuss specific features.


## How
Suppose you have ZIP file containing a `composer.json` with the name `<vendor>/<package-name>` and version `<package-version>`. All other fields are not validated, so make sure that the [`composer.json` is valid](https://getcomposer.org/doc/04-schema.md). All fields will be taken into account in the generated `packages.json`. Note that `<package-version>` does not need to be in the [semver](https://semver.org/) format, but any allowed folder name can be used.

The file system serves as "database", so that packages that should be included have to be placed in the folder `data/<vendor>/<package-name>/<package-version>`. The folder needs to contain two files, the `composer.json` which should be included in the `packages.json`, and a ZIP file `package.zip` containing the package content. Note that in principle the `composer.json` of that folder and inside the `package.zip` do not need to be equivalent. The API upload currently only supports these files to be the same, if there is a valid use-case please let me know, and we can discuss what is needed.

Additionally I wanted to test a small project in Go.


## Usage
There are two pairs of a username and password required, one for the user and one for the admin [basic authentication](https://datatracker.ietf.org/doc/html/rfc7617). The user credentials are needed for downloading packages and the admin credentials for uploading packages and they are read from the four environment variables `{USER,ADMIN}_AUTH_USERNAME` and `{USER,ADMIN}_AUTH_PASSWORD`. Currently they are all required. While for the admin API this will probably never change, the user authentication might become optional in the future. The `packages.json` is currently always accessible without any credentials.


### Example setup
First clone the repository and run inside corresponding folder the build
```
$ go build
```
which should create an executable file `composer-packagist`.

Create a `.env` file containing the authentication credentials
```
USER_AUTH_USERNAME=<package-user>
USER_AUTH_PASSWORD=<package-password>
ADMIN_AUTH_USERNAME=<admin-user>
ADMIN_AUTH_PASSWORD=<admin-password>
```

Source the `.env`-file and run the application
```
$ source .env
$ ./composer-packagist
```

Upload a package using
```
$ curl -u <admin-user>:<admin-password> --data-binary @my-package.zip http://localhost:3000/admin/upload
```
and download it as `<vendor>-<package-name>.zip` using
```
$ curl -u <package-user>:<package-password> --remote-header-name -O http://localhost:3000/package/<vendor>/<package-name>/<package-version>
```

To use composer to install the package create a new directory somewhere and run inside that
```
$ echo "{}" > composer.json
$ composer config secure-http false  # Only needed if there is no SSL certificate, e.g. for the local web server
$ composer config repositories.foobar-test '{"type": "composer", "url": "http://localhost:3000"}'
$ composer require <vendor>/<package-name>:<package-version>
```
Note that if you deploy this on a web-server, you probably want to use a reverse proxy for SSL termination. Currently there is the limitation that the generated package URL is always with `http`, which will be fixed in the future, or feel free to create a pull request.

### Secure connection
The environment variable `IS_SECURE={1,true}` can be used to specify if the `https` protocol should be used, but no TLS termination is performed, so it should be hosted behind a HTTP proxy.


## API Endpoints
### `GET /packages.json`
Get all packages which are found inside the `data` directory.

### `GET /package/:vendor/:package-name/:package-version`
Download a package ZIP containing the package content for the given parameters.

### `POST /admin/upload`
Upload a package where the payload of the request should contain the ZIP file.
