### Tidal
`FollowPlaylist` receiver method returns 3 values. Its the equivalent of `spotify.FetchPlaylistHash` and `spotify.FetchPlaylistSearchResult` together.
 Why not follow same pattern?

This is because Tidal does not use hash for playlist. We're checking
if the playlist has been updated by comparing the timestamp with the cached one. So that means that for the most part,
splitting them out means repeating similar code and while this isn't a problem, we might call it different places around same time.
that is, multiple trips even though we already got it first when we fetched the playlist.


### Services
`HasplaylistBeenUpdated` receiver method returns 4(!) values. This is because:
 - there is need to return "snapshotID" for playlists, but not all platforms use hashes (tidal uses timestamp, see Tidal section).
 - there is need to return the platform. This is because for tidal, we need to check if the playlist is tidal, we want to store the timestamp (as value of snapshotID), not the hash.
 - we need to return a boolean value to know if the playlist has been updated.
 - we need an error to know why it failed.

Probably this would've made sense to return a tuple... or struct but.. 1, go doesn't have tuples and 2... mvp and "it aint broke, worry about it later" baybee.
It should also be noted that the  format for the redis cache is:
"tidal"

#!/bin/bash

0-alias creates a new alias for a command 'rm *' which removes all files in current directory



## Suggested improvements / Future ideas to explore
 - [ ] Adding the "entity_id" to the notification info. perhaps a new column called "entity_id" in the notifications table. This would allow us to link to the entity in the notification. This would be useful for when we want to link to a playlist conversion, etc. notification.
 - [ ] Returning an array of users in the request body that already follow a playlist
 - [ ] Add when a playlist was updated to the meta info about a playlist follow update.
 - [ ] Support fetching from twitter circle/list* (this is a stretch goal??)
 - [ ] Notification endpoints
 - [ ] implement auth redirect URL for developers instead of manually setting in env var for Zoove