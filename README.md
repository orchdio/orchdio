## ORCHDIO

Orchdio is an API first platform for cross-platform streaming services apps. The goal is to provide a simple and easy to use API for streaming services.

The current streaming platforms all do the same thing which is to stream music — and many people use different accounts on each of these platforms. This poses
a problem for both users and developers; developers have to write the same code to use streaming music api on each platform, and users have to maintain separate accounts on each platform, Orchdio solves this by providing cross
platform APIs that enable developers build on various streaming services at once and allow users take charge of their accounts from several services
in one place.


## Feature Goals
 - Cross-platform track conversion: convert tracks from one platform to another
 - Cross-platform playlist conversion: convert playlists from one platform to another
 - Cross-platform account management: manage accounts from one platform to another
   - cross update of playlists — users can add a track on Deezer, to their playlist (using Orchdio) and have it automatically added on other platforms
 - Follow playlists: when a playlist is followed, anytime the playlist is updated (tracks added, removed, etc.), the user is automatically updated.
 - Cross-platform album conversion: convert albums from one platform to another
 - Playlist and track metadata and management APIs: allow developers manage playlist and tracks (and/or get their metadata).
For example, with an Orchdio playlist API, developers can build an app that allows user mass update their playlists and even build powerful
products like organic music discovery through what friends are playing and have on their playlists.


## Features
 - Track conversion
    - Tidal, Deezer and Spotify are currently supported
 - Playlist conversion
    - Tidal, Deezer and Spotify are currently supported
 - Playlist follow (WIP)
    - Tidal, Deezer and Spotify are currently supported
    

Additionally, the following APIs are available but not necessarily categorized as features since they are not directly for 
streaming services, but are Orchdio APIs:
 - Webhook API: developers get webhook events when
   - a long-running playlist conversion is done

_In the future, webhooks will be used for more events/actions and the above list will be expanded. Webhooks are the delivery lifeblood of events
that are happening or real-time updates with developer._

- Webhook management: create, update, delete and validation.
- User Authentication and Authorization — connecting user account to a platform:
  - Only Deezer and Spotify are supported
 
## WIP Features
 - Playlist and Track conversion support for Apple Music and YouTube.


## Overall Goal
The whole goal of orchdio is to provide a cross-platform streaming service API that allows developers to build on various streaming services at once and allow users take charge of their accounts from several services in one place.
Orchdio aims to make cross-platform streaming service APIs painless and easy to use.


## Design Guideline
Orchdio is derived from "Orchestrator" and "Audio" and it's also a play on "Audio". Design guideline is to present orchdio (visually) as connecting multiple services to create a single experience.

Some suggested (not necessarily needed if it won't work) colors include purple or blue.