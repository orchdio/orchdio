## ORCHDIO

Orchdio is an API first platform for cross-platform streaming services apps. The goal is to provide a simple and easy to use API for streaming services.

The current streaming platforms all do the same thing which is to stream music — and many people use different accounts on each of these platforms. This poses
a problem for both users and developers; developers have to write the same code to use streaming music api on each platform, and users have to maintain separate accounts on each platform, Orchdio solves this by providing cross
platform APIs that enable developers build on various streaming services at once and allow users take charge of their accounts from several services
in one place.

Currently, Orchdio supports the following streaming services:
- Tidal
- Deezer
- Spotify
- Apple Music
- YouTube Music

However, not all the platforms above support the same features. For example, TIDAL doesnt support login currently and
YouTube Music doesn't support playlist conversion. When a playlist is converted, at the moment, conversion can be done for TIDAL
but no way to add to account like Spotify, Tidal and Apple Music. 


### Features

 - Track conversion
    - All platforms are supported
 - Playlist conversion
   - TIDAL, Spotify, Apple Music, Deezer are currently supported
   - YouTube Music is currently not supported

 - Playlist follow
   - TIDAL, Spotify, Apple Music, Deezer are currently supported. If a user follows a playlist, a notification is sent when the playlist is updated. This is currently an API only feature.


These are core features of the platform that currently are working and are in beta testing. Currently,
developers can get access to API keys and start building apps based on the available APIs but as there are still things
that I am still trying to square away, I am manually talking to one or two developers to get started and get feedback.

Additional developer APIs available are:
 - Webhook
   - Developers can set webhooks and get notified when there is an action or update. Currently, this is for playlist updates.
 - User profile
   - Developers can get access to the user profile across platforms. This enables building really rich experiences.


## Zoove
Zoove is the flagship platform built on Orchdio. Its a platform that at its core, allows people get links to a track or playlist on various streaming platforms.

Currently, the following features are live
 - Track conversion
 - Playlist conversion
 - Connecting accounts on various platforms
   - users can sign in with either Spotify, Deezer or Apple Music. Technically, there is no idea of "Login" because Zoove
 auths users with their accounts on various platforms. They can also choose any account to switch to at any point. This is key
 and really one of the whole idea Zoove shines at.
 - Add a track to a playlist (after conversion) if signed in.

## Currently, WIP features
- Playlist follow
- Notifications
- User profile

Currently, the design for Zoove is limited and a little behind the API features. The idea is to redesign with the current and pending features in mind.
Users can currently connect account but that's about it. There are no notification system, no way to follow playlists, etc.

Also in the future, on the features on the roadmap that I have in mind would be to have a way to list all playlists, depending on
which platform the user is currently connected to. This unlocks the ability to build great playlist management experiences.

Some playlist management "experiences" are:
 - Global playlists — you create a playlist, your friends using other platforms can add to it (and followers get notified! how amazing and convenient is that?)


# Zoovebot
Zoovebot is a bot that is built on the Zoove API. It is a bot that allows users to get links to tracks and playlists on various streaming platforms.
Currently implemented on Twitter.

## Cha-Cha
Cha-Cha is a bot that is built on the Orchdio API. It is a bot that allows users to get links to tracks and playlists on various streaming platforms.
Currently implementing on Slack. This bot is inspired by Paystack music. It (will):
 - Allow users to get links to tracks and playlists on various streaming platforms
 - Auto-Create playlists (for various platforms) every month.
 - Advanced analytics based on sharing history, data, etc. E.g most played genre by week and team.

Like Zoovebot, it's open source but currently not available on GitHub (unlike Zoovebot).

Ideally, these open-source implementations and sub-products of Orchdio will be a good way to find a proper engineering help to help build the whole platforms, since
they're written in Go/Rust/JavaScript and are core languages I use and anybody who writes these or knows these, would be great to have on the team when the time comes.


## Roadmap
- 100 users (lmao)
- Playlist follow
- Notifications
- User profile
- Global playlists
- Add track to a playlist