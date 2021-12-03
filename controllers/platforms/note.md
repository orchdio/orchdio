## Redis Key-Values
Single track:
#
When a new track is being converted, first, we're checking inside the cache
with the key in the format `<platform>-<track_id>`. If the key exists, we're
then deserializing and returning. Note that the value of this key from the cache
has the same data as the one we're "setting by hash of title and artiste"

"Setting by hash of title and artiste"
#
This is the md5 hash of the title and artiste of the track. This is used because in some places
we're searching by just the title of the track and the artiste. 

For example, when a track is being converted from Deezer to Spotify, we first get the track
from Deezer, then we use the information (title and artiste) from it to search on Spotify. That means
that even though we may have the spotify record of the track before in cache (for example, someone else converted same track but from spotify instead),
we cannot return it because we dont know the `track-id`, so that means we'll have to search by title on Spotify.

In order to be able to fetch from cache in this case, thats why we're using the Hash of title and artiste. And why has, not just title and artiste?
An hash because we want to "sanitize" the key —handle special characters and also be able to not have to remember format of key