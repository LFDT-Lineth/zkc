;;error:3:12-14:malformed handle
(defcolumns (X :i16) (Y :i16) (one :i1 :constant 1))
(deflookup () ((one X)) ((one Y)))
