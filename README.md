## ORCHDIO

Orchdio is an API first platform for cross-platform streaming services apps. The goal is to provide simple and easy to use unified APIs for digital streaming platforms.

The current streaming platforms all do the same thing which is to stream music — and many people use different accounts on each of these platforms. This poses
a problem for both users and developers; developers have to write the same code to use music streaming api on each platform, and users have to maintain separate accounts on each platform, Orchdio solves this by providing unified, cross-platform APIs that enable developers build on various streaming services at once and allow users take charge of their accounts from several services in one place.


### You can find the developer documentation [here](https://orchdio-labs.gitbook.io/orchdio-api-documentation/)


### Apps and integrations
Orchdio is currently integrated into a few applications at the moment, most notably [Zoove](https://zoove.orchdio.com). Additional applications and integrations are currently in the works, including improvements to Zoove. Please [reach out](mailto:onigbindeayomide@gmail.com?subject=Orchdio%20integration%20chat) if you'd like to integrate Orchdio and need some help or need to chat.


### Sample API request
![Sample Screenshot](track-conversion.png)

### Building and running the project locally

Dependencies:
 - Go 1.24
 - Redis
 - Postgres
 - Svix (Webhook provider)

Set the environment variable `ORCHDIO_ENV=dev`. The application looks for a `.env.dev` file in the root directory, if the environment variable value is
dev. Otherwise, the application looks for a `.env` file instead. Then run the following commands:
```bash
 $ export ORCHDIO_ENV=dev
 $ go build -o cmc/orchdio && ./cmd/orchdio
   ```


Please check the `.env.example` file to see the possible env various needed and their suggested values.

```txt
 ⚠️ You'd need to setup [Svix](https://www.svix.com). This is a Webhook as a Service provider and used as the supported webhook delivery platform in Orchdio. Please follow the documentation to get started. You can use Orchdio without setting up Svix but you'll not be able to get Webhook events on the status of your conversions and other actions. This means that for playlist conversion for example, you could poll an endpoint to get the results you want though. Please check the documentation for more information.
```

#### Running with Docker
You can build and run with docker. By default, Orchdio uses `orchdio` db in the docker image as the DB. You can find this in the `docker-entry.sh` file at the root of the project. If you want to change your DB url being used in Docker, this is where you find it.

As specified above, you can still run Orchdio without Docker. simply ensure that the appropriate env is set (ORCHDIO_ENV) with the corresponding .env file.


### Testing
Testing is an interesting problem for Orchdio because at its core, Orchdio relies heavily on 3rd party APIs and makes calls to these APIs a lot. The implication of this is that in order to have tests, integration tests would make a lot more sense; however this is a bit tricky because that means needing to do A LOT of mocking of these APIs.

The other tests would ideally be unit tests, which are largely redundant (IMO), eg testing that data is saved and gotten from the database. This means that ideally integration tests might be a bit beneficial for, but also a little be stressful due to having to Mock and maintain these mocks which WILL grow as time goes on.

That is NOT to say there are no tests in place. Currently, there are some integration tests setup which demonstrate how testing can be done on Orchdio. Help on extending this would be very much appreciated. What this means is that NOT everything would be tested, only important parts would be tested.

There are integration tests currently setup under the `tests/` folder. Importantly, `ConvertTrack` (which as it sounds, handles track conversions) is currently tested. With time, coverage is expected to extend to things like `ConvertPlaylist` which is a bit more complex.

**THE APIS DO WORK AS EXPECTED THOUGH**


#### Helpful tools.
The following could be helpful in development:
- [Asynqmon](https://github.com/hibiken/asynqmon): This is a web UI for the [Asynq](https://github.com/hibiken/asynq) task queue used for working with queues.
- [Tunnelto](https://tunnelto.dev/) : for exposing local servers over the internet using a custom subdomain, over HTTPs. This is useful when working with webhooks.
