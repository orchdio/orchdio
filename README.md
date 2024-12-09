## ORCHDIO

Orchdio is an API first platform for cross-platform streaming services apps. The goal is to provide simple and easy to use unified APIs for digital streaming platforms.

The current streaming platforms all do the same thing which is to stream music — and many people use different accounts on each of these platforms. This poses
a problem for both users and developers; developers have to write the same code to use streaming music api on each platform, and users have to maintain separate accounts on each platform, Orchdio solves this by providing unified, cross-platform APIs that enable developers build on various streaming services at once and allow users take charge of their accounts from several services
in one place.


#### Building and running the project

Requirements:
 - Go 1.17
 - Redis
 - Postgres

Set the environment variable `ORCHDIO_ENV=dev`. The application looks for a `.env.dev` file in the root directory, if the environment variable value is
dev. Otherwise, the application looks for a `.env` file instead. Then run the following commands:   


The following could be helpful in development:
- [Asynqmon](https://github.com/hibiken/asynqmon): This is a web UI for the [Asynq](https://github.com/hibiken/asynq) task queue used for working with queues.
- [Tunnelto](https://tunnelto.dev/) : for exposing local servers over the internet using a custom subdomain, over HTTPs. This is useful when working with webhooks.


```bash

 $ export ORCHDIO_ENV=dev 
 $ go build -o cmc/orchdio && ./cmd/orchdio
   ```