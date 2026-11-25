;;error:3:23-24:expected u16, found u17
(defcolumns (X :i17) (Y :i16) (one :i1 :constant 1))
(deflookup test ((one X)) ((one Y)))
